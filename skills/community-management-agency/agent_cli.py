#!/usr/bin/env python3
"""KERN_AGENT_CLI adapter for the community-management-agency graph
(examples/community-management-agency.yaml).

One process per node invocation — agentrunner.Subprocess's protocol (see
Kern-Orch/internal/agentrunner/protocol.go): one JSON request on stdin
({"node_id","prompt","state"}), one JSON-lines event on stdout, the last "result" event
wins. No interactivity: kern-orch's own ApprovalNode is what pauses the graph for a human
decision at confirm_strategie/confirm_publication — this script never blocks on input.

Shells out to the real `claude` CLI (Claude Code), exactly like
skills/prospection/agent_cli.py — see that file's docstring for why (no separate LLM
API, no LangChain machinery, `claude -p` runs to completion and returns finished text).

Reuses crew-comm's already-written prompts (plain text constants, no LangChain coupling)
as a library — Kern is the demo, crew-comm a proven source of prompt engineering, not a
parallel system running alongside it. `_extraire_plan` (redacteur.py) is imported the
same way prospection imports `_marquer_inventions` from crew-crm: private by naming
convention, reused anyway, same precedent.

`graph.State` wire shape (Kern-Orch/internal/graph/state.go, MarshalJSON): the state
object on the wire is {"step":N,"frozen":N,"data":{...},"zones":{...}} — every value an
agent node reads or writes lives under "data", not at the top level.
"""
import json
import re
import subprocess
import sys
from datetime import date
from pathlib import Path

CREW_COMM = Path("/Users/yoann/Developer/mon-orchestrateur-agents/agents-locaux/crew-comm")
sys.path.insert(0, str(CREW_COMM))

# Prompt text and pure string helpers only — no LangChain, no model wiring imported.
from agents.audience import PROMPT as AUDIENCE_PROMPT  # noqa: E402
from agents.strategiste import PROMPT as STRATEGISTE_PROMPT  # noqa: E402
from agents.redacteur import PROMPT as REDACTEUR_PROMPT  # noqa: E402
from agents.redacteur import RAPPEL_FORMAT as REDACTEUR_RAPPEL_FORMAT  # noqa: E402
from agents.redacteur import _extraire_plan  # noqa: E402

CLAUDE_TIMEOUT_S = 180

# 2026-08-05, décision : crew-comm n'a qu'un mode (le stratège propose toujours). Le
# double mode (avis sur une stratégie déjà fournie par l'utilisateur / proposition par le
# stratège) est nouveau pour ce skill — ajouté ici comme une instruction supplémentaire,
# pas en modifiant crew-comm/agents/strategiste.py, pour rester une réutilisation en
# bibliothèque plutôt qu'une divergence silencieuse entre les deux systèmes.
STRATEGISTE_MODE_INSTRUCTION = """Avant toute chose, détermine le MODE de ce tour et
commence ta réponse par UNE SEULE ligne, exactement sous cette forme :
"MODE: avis" si la demande de l'utilisateur contient déjà une stratégie complète (angle,
plateforme, ton, moment) qu'il te demande seulement de commenter — dans ce cas tu donnes
un avis court (5 lignes maximum : ce qui fonctionne, ce qui manque, ce que tu changerais)
et tu NE PRODUIS PAS de section "BRIEF ÉDITORIAL".
"MODE: proposition" dans tous les autres cas — tu proposes alors la stratégie toi-même,
selon les règles ci-dessous, en terminant par la section "BRIEF ÉDITORIAL" comme demandé.
"""

MODE_RE = re.compile(r"^\s*MODE\s*:\s*(avis|proposition)", re.IGNORECASE)

# 2026-08-06, décision : crew-comm ne connaît que les réseaux sociaux (le prompt du
# stratège liste explicitement "LinkedIn, Instagram, X, TikTok..."). L'email froid n'y
# figure pas du tout — ajouté ici en instruction supplémentaire, même technique que
# STRATEGISTE_MODE_INSTRUCTION, sans toucher à crew-comm/agents/strategiste.py.
STRATEGISTE_EMAIL_INSTRUCTION = """Une plateforme supplémentaire existe, en plus de celles
déjà citées : "email froid" (prospection écrite, outreach). Si elle est pertinente pour la
demande, tu peux la choisir comme plateforme — avec ses propres codes, différents de
LinkedIn : objet court et concret (jamais putaclic), corps de 3 à 6 phrases, un seul
call-to-action, aucun emoji, ton direct. Si une relance à plusieurs touches est pertinente,
propose un nombre de touches (2 à 4) espacées dans le temps plutôt qu'un email unique —
précise l'espacement (ex. "J+3", "J+7") dans le brief éditorial."""

# Le squelette de plan de redacteur.py (RAPPEL_FORMAT) n'accepte que les lignes commençant
# par "Publier"/"Programmer"/"Planifier" (VERBES_ACTION, crew-comm/agents/redacteur.py) —
# _extraire_plan filtre tout le reste. Plutôt que dupliquer cette extraction avec un
# quatrième verbe, l'email réutilise le même verbe "Publier" au prix d'une légère bizarrerie
# de phrasé ("Publier l'email...") : garde une seule logique d'extraction, zéro divergence
# avec crew-comm.
REDACTEUR_EMAIL_INSTRUCTION = """Si le brief éditorial indique la plateforme "email froid" :
- Rédige une ligne "Objet : ..." avant le corps du message.
- Corps court (3 à 6 phrases), un seul call-to-action, aucun lien de tracking inventé.
- Si le brief demande plusieurs touches, rédige chaque touche séparément ("Touche 1",
  "Touche 2"...), chacune avec son propre objet et corps.
- Le squelette de plan reste inchangé : chaque ligne commence quand même par "Publier",
  "Programmer" ou "Planifier" — ex. "Publier l'email à <destinataire> le <date> : <objet>"."""


def emit_result(output: dict) -> None:
    print(json.dumps({"type": "result", "output": output}), flush=True)


def emit_error(message: str) -> None:
    print(json.dumps({"type": "error", "message": message}), flush=True)


def run_claude(prompt: str) -> str:
    """Runs `claude -p` to completion and returns its plain-text answer. No MCP/tool
    access needed for this graph's nodes yet — content drafting and review, not action
    on an external system (that's what confirm_publication gates)."""
    args = ["claude", "-p", prompt, "--output-format", "text"]
    result = subprocess.run(
        args, stdin=subprocess.DEVNULL, capture_output=True, text=True, timeout=CLAUDE_TIMEOUT_S
    )
    if result.returncode != 0:
        raise RuntimeError(f"claude exited {result.returncode}: {result.stderr[:500]}")
    return result.stdout.strip()


def run_audience(data: dict) -> dict:
    message = data.get("message", "")
    prompt = f"{AUDIENCE_PROMPT.format(regle_cadrage='')}\n\n--- DEMANDE ---\n{message}"
    brief = run_claude(prompt)
    return {"audience_context": brief}


def extract_mode(content: str) -> str:
    """Reads the mandatory leading "MODE: avis|proposition" marker. Missing or
    unrecognized defaults to "proposition" — the safe default, since onStrategyMode
    (internal/cmd/comm_routers.go) routes anything but a literal "avis" to the human
    approval gate rather than skipping it."""
    m = MODE_RE.match(content or "")
    return m.group(1).lower() if m else "proposition"


def strip_mode_line(content: str) -> str:
    return MODE_RE.sub("", content or "", count=1).strip()


def run_strategiste(data: dict) -> dict:
    audience = data.get("audience_context", "")
    message = data.get("message", "")
    system = STRATEGISTE_PROMPT.format(
        aujourd_hui=date.today().strftime("%A %d %B %Y"),
        regle_cadrage="",
        playbook="",
        audience=audience,
    )
    prompt = (
        f"{STRATEGISTE_MODE_INSTRUCTION}\n\n{STRATEGISTE_EMAIL_INSTRUCTION}\n\n{system}"
        f"\n\n--- DEMANDE DE L'UTILISATEUR ---\n{message}"
    )
    content = run_claude(prompt)

    mode = extract_mode(content)
    reste = strip_mode_line(content)
    output = {"mode": mode}
    if mode == "avis":
        output["avis_strategie"] = reste
        # Sans BRIEF ÉDITORIAL produit en mode avis, le rédacteur retombe sur la
        # stratégie telle que l'utilisateur l'a formulée — jamais réinventée ici.
        output["brief_editorial"] = message
    else:
        output["brief_editorial"] = reste
    return output


def run_redacteur(data: dict) -> dict:
    brief = data.get("brief_editorial", "(aucun brief éditorial disponible)")
    system = REDACTEUR_PROMPT.format(
        aujourd_hui=date.today().strftime("%A %d %B %Y"),
        rappel_format=REDACTEUR_RAPPEL_FORMAT,
        playbook="",
        brief=brief,
    )
    prompt = f"{REDACTEUR_EMAIL_INSTRUCTION}\n\n{system}"
    content = run_claude(prompt)
    plan = _extraire_plan(content)
    return {"plan_propose": plan, "texte_redige": content}


def run_publieur(data: dict) -> dict:
    decision = data.get("decision:confirm_publication", "")
    plan = data.get("plan_propose", "")
    if decision != "approve":
        return {"execution": "Refusé par l'utilisateur : rien n'a été publié."}
    if not plan:
        return {"execution": "Aucun plan de publication à exécuter."}
    # G2 de crew-comm/agents/publieur.py, reproduit à l'identique : sans connecteur de
    # publication branché (aucun --mcp-config ici), on n'appelle même pas le modèle —
    # c'est la seule façon sûre d'empêcher un faux compte-rendu de publication.
    return {
        "execution": (
            "⚠️ Aucun connecteur de publication n'est branché : plan validé mais NON "
            f"exécuté.\n\n{plan}"
        )
    }


NODE_HANDLERS = {
    "audience": run_audience,
    "strategiste": run_strategiste,
    "redacteur": run_redacteur,
    "publieur": run_publieur,
}

# What kern-ui's hive graph shows when someone clicks that node (same convention as
# skills/prospection/agent_cli.py's DISPLAY_KEYS — state["display:<nodeId>"]).
DISPLAY_KEYS = {
    "audience": "audience_context",
    "strategiste": "brief_editorial",
    "redacteur": "texte_redige",
    "publieur": "execution",
}


def main() -> None:
    req = json.loads(sys.stdin.readline())
    node_id = req["node_id"]
    data = (req.get("state") or {}).get("data") or {}

    handler = NODE_HANDLERS.get(node_id)
    if handler is None:
        emit_error(f"unknown node_id: {node_id}")
        return
    try:
        output = handler(data)
    except Exception as e:
        emit_error(str(e))
        return

    headline_key = DISPLAY_KEYS.get(node_id)
    if headline_key and output.get(headline_key):
        output[f"display:{node_id}"] = output[headline_key]

    emit_result(output)


if __name__ == "__main__":
    main()
