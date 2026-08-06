#!/usr/bin/env python3
"""KERN_AGENT_CLI adapter for the courtage-extraction graph
(examples/courtage-extraction.yaml).

Same subprocess-per-node-invocation protocol as
skills/community-management-agency/agent_cli.py (see that file's docstring for the wire
shape and the reasoning behind shelling out to `claude -p`) — this file follows the exact
same conventions, no new pattern invented.

Pipeline (see Kern-Orch/skills/specs.md, "Besoin #1 — Extraction documentaire") :
  reception (ce fichier) -> extraction (ce fichier, boucle interne page par page)
  -> anonymizePII (Go builtin, internal/cmd/courtage_anon.go) -> interprétation (ce
  fichier, Claude ne voit jamais le texte en clair) -> deanonymizePII (Go builtin)
  -> confirm_extraction (validation humaine, kern-orch).

OCR : deux moteurs switchables, même patron que send_telegram/send_x (vide = repli sûr,
configuré = vrai appel) — voir pick_ocr_engine(). Un texte déjà présent dans la couche
texte du PDF (fitz.Page.get_text()) n'est JAMAIS envoyé à un OCR, local ou cloud : c'est
une optimisation ET une garantie de sobriété (rien n'est transmis à un tiers si ce n'est
pas nécessaire).
"""
import json
import os
import re
import subprocess
import sys
import urllib.error
import urllib.request
from pathlib import Path

import fitz  # PyMuPDF
import pytesseract
from PIL import Image

CLAUDE_TIMEOUT_S = 180
MISTRAL_OCR_URL = "https://api.mistral.ai/v1/ocr"
OCR_DPI = 200

IMAGE_EXTS = {".png", ".jpg", ".jpeg", ".tif", ".tiff"}

JSON_FENCE_RE = re.compile(r"^```(?:json)?\s*|\s*```$", re.MULTILINE)


def pick_ocr_engine() -> str:
    """"Vide = repli sûr, configuré = vrai appel" — même patron que
    community-management-agency/agent_cli.py (TELEGRAM_BOT_TOKEN/X_API_KEY). Aucune clé
    Mistral dans l'environnement -> moteur local Tesseract, zéro dépendance externe."""
    return "mistral" if os.environ.get("MISTRAL_API_KEY") else "tesseract"


def strip_json_fence(text: str) -> str:
    return JSON_FENCE_RE.sub("", text.strip()).strip()


def emit_result(output: dict) -> None:
    print(json.dumps({"type": "result", "output": output}), flush=True)


def emit_error(message: str) -> None:
    print(json.dumps({"type": "error", "message": message}), flush=True)


def run_claude(prompt: str) -> str:
    args = ["claude", "-p", prompt, "--output-format", "text"]
    result = subprocess.run(
        args, stdin=subprocess.DEVNULL, capture_output=True, text=True, timeout=CLAUDE_TIMEOUT_S
    )
    if result.returncode != 0:
        raise RuntimeError(f"claude exited {result.returncode}: {result.stderr[:500]}")
    return result.stdout.strip()


def ocr_image_local(image: Image.Image) -> str:
    return pytesseract.image_to_string(image, lang="fra")


def ocr_image_mistral(image_bytes: bytes, api_key: str) -> str:
    """Real Mistral OCR call — pure text extraction (the /v1/ocr endpoint, not a chat
    completion), so this is still "OCR pur" per the pipeline's ordering constraint: no
    interpretive model touches the document before kern-anon has masked its text."""
    import base64

    payload = json.dumps(
        {
            "model": "mistral-ocr-latest",
            "document": {
                "type": "image_url",
                "image_url": f"data:image/png;base64,{base64.b64encode(image_bytes).decode()}",
            },
        }
    ).encode()
    req = urllib.request.Request(
        MISTRAL_OCR_URL,
        data=payload,
        method="POST",
        headers={
            "Authorization": f"Bearer {api_key}",
            "Content-Type": "application/json",
        },
    )
    try:
        with urllib.request.urlopen(req, timeout=60) as resp:
            body = json.loads(resp.read())
    except (urllib.error.URLError, OSError) as e:
        raise RuntimeError(f"Mistral OCR injoignable : {e}") from e

    pages = body.get("pages") or []
    if not pages:
        raise RuntimeError(f"Mistral OCR a répondu sans page : {body}")
    return "\n".join(p.get("markdown", "") for p in pages).strip()


def _extract_page(page: "fitz.Page") -> tuple[str, bool]:
    """Returns (text, used_ocr) for one page — direct text layer first, OCR only when the
    page has none (a scanned image page)."""
    text = page.get_text().strip()
    if text:
        return text, False

    pix = page.get_pixmap(dpi=OCR_DPI)
    img_bytes = pix.tobytes("png")
    if pick_ocr_engine() == "mistral":
        text = ocr_image_mistral(img_bytes, os.environ["MISTRAL_API_KEY"])
    else:
        import io

        text = ocr_image_local(Image.open(io.BytesIO(img_bytes)))
    return text.strip(), True


def extract_text_from_pdf(path: str) -> dict:
    """Chunking by page lives here, inside this single node's implementation — kern-orch's
    graph has no native loop construct, and none is needed (specs.md, "Découpage par page
    dans le graphe kern-orch"). A failure on one page does not need to sink the whole
    document; pages are processed independently."""
    doc = fitz.open(path)
    try:
        parts = []
        pages_ocr = []
        for i in range(len(doc)):
            text, used_ocr = _extract_page(doc[i])
            parts.append(f"[page {i + 1}]\n{text}")
            if used_ocr:
                pages_ocr.append(i + 1)
        return {"text": "\n\n".join(parts), "pages": len(doc), "pages_ocr": pages_ocr}
    finally:
        doc.close()


def extract_text_from_image(path: str) -> dict:
    if pick_ocr_engine() == "mistral":
        text = ocr_image_mistral(Path(path).read_bytes(), os.environ["MISTRAL_API_KEY"])
    else:
        text = ocr_image_local(Image.open(path))
    return {"text": f"[page 1]\n{text.strip()}", "pages": 1, "pages_ocr": [1]}


def run_reception(data: dict) -> dict:
    # "message" is what kern-orch's dispatch nudge actually populates (see
    # internal/cmd/serve.go, mailbox.Nudge("message", text)) — the chat text IS the
    # document path for this first pass (specs.md, "Ingestion des documents": seul le
    # canal chemin/dossier est câblé). "document_path" stays accepted too, for a future
    # caller that sets state directly instead of going through chat text.
    path = (data.get("document_path") or data.get("message") or "").strip()
    if not path:
        raise RuntimeError(
            "document_path manquant : aucun document fourni (voir specs.md — seul le "
            "canal chemin/dossier est câblé pour l'instant)."
        )
    if not Path(path).exists():
        raise RuntimeError(f"document_path introuvable : {path}")
    return {"document_path": path}


def run_extraction(data: dict) -> dict:
    path = data.get("document_path", "")
    ext = Path(path).suffix.lower()
    if ext == ".pdf":
        result = extract_text_from_pdf(path)
    elif ext in IMAGE_EXTS:
        result = extract_text_from_image(path)
    else:
        raise RuntimeError(f"format non supporté : {ext or '(aucune extension)'}")

    return {
        "extracted_text": result["text"],
        "extraction_pages_ocr": result["pages_ocr"],
        "display:extraction": (
            f"{result['pages']} page(s) traitée(s), OCR utilisé sur "
            f"{len(result['pages_ocr'])} page(s)."
        ),
    }


INTERPRETATION_PROMPT = """Tu es un analyste de dossier de rachat de crédit. On te donne
un texte extrait de documents (avis d'imposition, relevés bancaires, tableaux
d'amortissement), dans lequel les données personnelles ont déjà été remplacées par des
jetons comme <IBAN_1>, <TELEPHONE_1>, <EMAIL_1>, <PII_1> : NE JAMAIS inventer de valeur
pour ces jetons, recopie-les tels quels partout où l'information originale apparaîtrait.

Réponds UNIQUEMENT avec un objet JSON valide, sans texte autour, de cette forme exacte :
{
  "revenus": [{"source": "...", "montant_mensuel": <nombre ou null>, "document_source": "...", "statut": "confirmé"|"à vérifier"}],
  "credits_en_cours": [{"etablissement": "...", "mensualite": <nombre ou null>, "capital_restant_du": <nombre ou null>, "document_source": "...", "statut": "confirmé"|"à vérifier"}],
  "incidents": [{"type": "...", "date": "...", "montant": <nombre ou null>, "document_source": "..."}],
  "reste_a_vivre": {"montant": <nombre ou null>, "methode_calcul": "...", "statut": "..."},
  "pieces_manquantes": ["..."]
}

Chaque enregistrement DOIT porter "document_source" (le numéro de page, ex. "page 2") et
un "statut" honnête. Ne jamais inventer un chiffre absent du texte : mets null et
"à vérifier" plutôt qu'une valeur affirmée sans preuve claire dans le texte."""


def run_interpretation(data: dict) -> dict:
    masked = data.get("masked_text", "")
    prompt = f"{INTERPRETATION_PROMPT}\n\n--- TEXTE (PII masqué) ---\n{masked}"
    raw = run_claude(prompt)
    cleaned = strip_json_fence(raw)
    try:
        json.loads(cleaned)
    except json.JSONDecodeError as e:
        raise RuntimeError(f"réponse non-JSON du modèle : {e}\n{raw[:300]}") from e

    return {
        "interpretation_masked": cleaned,
        "display:interpretation": "Analyse structurée générée (données encore masquées).",
    }


NODE_HANDLERS = {
    "reception": run_reception,
    "extraction": run_extraction,
    "interpretation": run_interpretation,
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

    emit_result(output)


if __name__ == "__main__":
    main()
