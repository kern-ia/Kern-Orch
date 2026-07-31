#!/usr/bin/env python3
"""KERN_AGENT_CLI adapter for the prospection graph (examples/prospection.yaml).

One process per node invocation — agentrunner.Subprocess's protocol (see
Kern-Orch/internal/agentrunner/protocol.go): one JSON request on stdin
({"node_id","prompt","state"}), one or more JSON-lines events on stdout, the last
"result" event wins. No interactivity: kern-orch's own ApprovalNode is what pauses the
graph for a human decision between "commercial" and "approved" — this script never
blocks waiting for input.

Reuses crew-crm's already model-agnostic config, prompts and tools AS A LIBRARY — Kern
is the demo, crew-crm is a proven source of logic, not a parallel system running
alongside it. See ~/mon-orchestrateur-agents/agents-locaux/crew-crm/config.py
(MODEL_BACKEND=ollama|gemini|anthropic) for the model switch used here unchanged.

`graph.State` wire shape (Kern-Orch/internal/graph/state.go, MarshalJSON): the state
object on the wire is {"step":N,"frozen":N,"data":{...},"zones":{...}} — every value an
agent node reads or writes lives under "data", not at the top level.
"""
import asyncio
import json
import os
import re
import sys
from datetime import date
from pathlib import Path

CREW_CRM = Path("/Users/yoann/mon-orchestrateur-agents/agents-locaux/crew-crm")
sys.path.insert(0, str(CREW_CRM))

# Loaded explicitly here, not left to mcp_client.charger_tools()'s side effect: that
# function is only called by secretaire/expert (they need CRM tools), never by
# commercial (web search only) — which would otherwise reach GOOGLE_API_KEY/
# ANTHROPIC_API_KEY-less every time, found by running the commercial branch for real.
from dotenv import load_dotenv  # noqa: E402

load_dotenv(CREW_CRM / ".env")

from agents.commercial import PROMPT as COMMERCIAL_PROMPT  # noqa: E402
from agents.commercial import _filtrer_actions, _marquer_inventions  # noqa: E402
from agents.expert import PROMPT as EXPERT_PROMPT  # noqa: E402
from agents.expert import _decouper_plan  # noqa: E402
from agents.outils import boucle_react  # noqa: E402
from agents.outils_web import TOOLS_WEB  # noqa: E402
from agents.secretaire import PROMPT as SECRETAIRE_PROMPT  # noqa: E402
from config import llm_fort, llm_rapide  # noqa: E402
from mcp_client import charger_tools, tools_lecture_seule  # noqa: E402


def _silence_stdout() -> int:
    """Redirects fd 1 to fd 2 for the rest of this process and returns a saved copy of
    the real fd 1. Needed because crew-crm's reused code (mcp_client.charger_tools,
    diagnostic prints in commercial.py/expert.py) writes plain text to stdout — a single
    non-JSON line there would break agentrunner.Subprocess's line-by-line JSON parser
    and abort the whole run. Done at the OS file-descriptor level, not by reassigning
    Python's sys.stdout, so it also catches C-extension output the LLM/MCP client
    libraries might produce."""
    saved = os.dup(1)
    os.dup2(2, 1)
    return saved


def _restore_stdout(saved: int) -> None:
    # Flush BEFORE restoring: stdout is block-buffered when piped (not a terminal), so
    # anything crew-crm's code printed while silenced is still sitting in a userspace
    # buffer, not yet written to fd 2 — restoring first would let it leak into the real
    # stdout the moment Python's buffer next flushes (e.g. on this script's own exit).
    sys.stdout.flush()
    os.dup2(saved, 1)
    os.close(saved)


def texte(content) -> str:
    """Normalizes a message's `.content` to plain text across providers: Ollama and
    Anthropic already return a string, Gemini returns a list of content blocks
    ({"type": "text", "text": "...", ...}) — found by running this for real against
    Gemini, not assumed. Anything else (a stray non-text block) is dropped rather than
    stringified, so a plan or a fiche never carries a raw Python repr."""
    if isinstance(content, str):
        return content
    if isinstance(content, list):
        return "".join(
            block.get("text", "") for block in content if isinstance(block, dict) and block.get("type") == "text"
        )
    return str(content)


def emit_result(output: dict) -> None:
    print(json.dumps({"type": "result", "output": output}), flush=True)


def emit_error(message: str) -> None:
    print(json.dumps({"type": "error", "message": message}), flush=True)


async def run_secretaire(data: dict) -> dict:
    message = data.get("message", "")
    tools = await charger_tools()
    lecture = tools_lecture_seule(tools)
    llm = llm_rapide()
    messages = [("system", SECRETAIRE_PROMPT), ("user", message)]
    nouveaux = await boucle_react(llm, lecture, messages, max_tours=6)
    return {"lead_context": texte(nouveaux[-1].content)}


async def run_commercial(data: dict) -> dict:
    fiche = data.get("lead_context", "")
    llm = llm_fort(temperature=0.3)
    fiche_bloc = f"--- FICHE PRÉPARÉE PAR LA SECRÉTAIRE ---\n{fiche}" if fiche else ""
    system = COMMERCIAL_PROMPT.format(
        aujourd_hui=date.today().strftime("%A %d %B %Y"), playbook="", fiche=fiche_bloc
    )
    # _filtrer_actions (below) only recognizes a line as an action if it starts with a
    # bullet or number — a hard requirement the original prompt states loosely ("une
    # action par ligne"). Found by testing against Gemini: it wrote the CRM plan as plain
    # paragraphs, not bullets, so every action was silently dropped. Reinforced here,
    # local to this adapter, rather than loosening the (deliberately strict) extraction.
    system += (
        "\n\nFORMAT OBLIGATOIRE pour la section PLAN D'ACTION CRM : chaque action sur "
        "sa propre ligne, précédée d'un tiret \"- \" — jamais un paragraphe."
    )
    messages = [("system", system), ("user", "Prépare un plan d'action pour ce lead.")]
    nouveaux = await boucle_react(llm, TOOLS_WEB, messages, max_tours=6)
    content = texte(nouveaux[-1].content)

    plan = ""
    m = re.search(r"plan\s+d['']action\s+crm\s*:?", content, re.IGNORECASE)
    if m:
        section = re.split(r"\n(?:---|###|\*\*Message)", content[m.end():], maxsplit=1)[0]
        plan = _filtrer_actions(section).strip(" :\n")
        if plan:
            plan, _ = _marquer_inventions(plan, corpus="")
    return {"plan_propose": plan, "compte_rendu_commercial": content}


async def run_expert(data: dict) -> dict:
    plan = data.get("plan_propose", "")
    decision = data.get("decision:confirm", "")
    if decision != "approve":
        return {"execution": "Refusé par l'utilisateur : rien n'a été exécuté."}
    if not plan:
        return {"execution": "Aucun plan à exécuter."}

    tools = await charger_tools()
    if not tools:
        return {"execution": "⚠️ Aucun outil CRM disponible : plan NON exécuté."}

    llm = llm_fort()
    prompt_expert = EXPERT_PROMPT
    member_id = os.environ.get("CRM_MEMBER_ID", "")
    if member_id:
        prompt_expert += f"\n- Quand un outil exige un memberId, utilise TOUJOURS : {member_id}"

    lignes = []
    for i, action in enumerate(_decouper_plan(plan), 1):
        demande = f"Action {i} à exécuter maintenant : {action}"
        messages = [("system", prompt_expert), ("user", demande)]
        nouveaux = await boucle_react(llm, tools, messages, max_tours=5)
        appels = [tc["name"] for m in nouveaux for tc in (getattr(m, "tool_calls", None) or [])]
        conclusion = texte(nouveaux[-1].content).strip()
        statut = "✅" if appels else "⚠️ (aucun outil appelé)"
        lignes.append(f"{statut} Action {i} : {action}\n   outils : {appels or 'aucun'}\n   {conclusion[:300]}")

    return {"execution": "\n\n".join(lignes)}


NODE_HANDLERS = {
    "secretaire": run_secretaire,
    "commercial": run_commercial,
    "approved": run_expert,
}


async def main() -> None:
    req = json.loads(sys.stdin.readline())
    node_id = req["node_id"]
    data = (req.get("state") or {}).get("data") or {}

    handler = NODE_HANDLERS.get(node_id)
    if handler is None:
        emit_error(f"unknown node_id: {node_id}")
        return

    saved_stdout = _silence_stdout()
    try:
        output = await handler(data)
    except Exception as e:
        _restore_stdout(saved_stdout)
        emit_error(str(e))
        return
    _restore_stdout(saved_stdout)
    emit_result(output)


if __name__ == "__main__":
    asyncio.run(main())
