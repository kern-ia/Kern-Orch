# CONVENTIONS.md — kern-orch

Autorité locale pour ce repo, comme annoncé par le [CONTRIBUTING.md](https://github.com/kern-ia/.github/blob/main/CONTRIBUTING.md)
de l'organisation. Les règles communes à tous les repos `kern-ia` sont reprises ci-dessous ;
la section « Spécificités » couvre ce qui n'appartient qu'à `kern-orch`.

## Branches

- `main` : branche stable, toujours déployable. Protégée — aucun push direct.
- `dev` : branche d'intégration. Protégée — aucun push direct.
- Branches de travail : `feature/<slug>`, `fix/<slug>`, `chore/<slug>`, `docs/<slug>`, `test/<slug>`.
- Toute modification de `main` ou `dev` passe par une Pull Request, jamais par un push direct
  ni un `git merge` local suivi d'un push.
- Merge vers `dev` : merge commit `--no-ff`.

> **Écart actuel à corriger** : la branche par défaut du repo GitHub est aujourd'hui `dev`,
> pas `main`. À changer dans Settings → Branches une fois `main` réellement à jour et protégée
> (voir rapport de conformité).

## Commits

Conventional Commits : `type(scope): résumé court`. Types déjà utilisés ici : `feat`, `fix`,
`docs`, `test`, `chore`, `merge`. Le corps explique le *pourquoi*. Aucune signature d'outil
(trailer `Co-Authored-By`, `Claude-Session` ou équivalent) dans les messages de commit —
l'auteur du commit git suffit.

## Pull Requests

- Un seul sujet par PR, liée à l'issue ou la RFC qu'elle résout.
- Template PR hérité de `kern-ia/.github`.
- Déclare l'impact semver.
- Aucune donnée personnelle réelle.

> **Écart actuel** : une seule PR GitHub existe sur ce repo (#1). Le flux réel est un
> `git merge` local suivi d'un push direct sur `dev` — donc sans revue possible sur GitHub.
> À corriger : tout changement doit désormais ouvrir une PR, même en solo, pour que la CI
> (une fois en place) et l'historique de revue existent.

## Style et lint

- `go vet ./...` obligatoire.
- Pas de `.golangci.yml` aujourd'hui — à ajouter, base `linters.default: standard`
  (voir `kern-anon` ou `kern-link` comme référence).

## Tests

- `go test ./...` doit être vert avant toute PR.
- Test unitaire ciblé : `go test ./internal/cmd/ -run <NomDuTest> -v`.

> **Écart actuel — le plus important** : ce repo n'a **aucun workflow GitHub Actions**.
> Rien ne vérifie build/vet/test/lint à la PR. À ajouter en priorité : un `.github/workflows/ci.yml`
> calqué sur celui de `kern-anon` (`go build`, `go test -race -cover ./...`, `golangci-lint`).

## Module Go

- Chemin actuel : `github.com/yoann/kern-orch` — même écart que `kern-ui`, à trancher au
  niveau de l'organisation plutôt que repo par repo.

## Architecture

- Dépendances à sens unique : `graph` définit les ports (`AgentRunner`, `StepFunc`) ;
  `agentrunner` et `checkpoint` dépendent de `graph`, jamais l'inverse. Toute PR qui inverse
  ce sens doit être justifiée explicitement dans sa description.
- Le protocole JSON-lines d'`agentrunner` est un placeholder assumé (spec §6.4) — ne pas le
  durcir en API stable sans revoir la spec.

## Documentation

- `README.md` à la racine.
- `CLAUDE.md` — contexte agent, section « Commands » à garder synchronisée avec les vraies
  commandes du Makefile / CLI.
- Index de features sous `docs/index/` (pattern OKF), à privilégier avant de relire tout le code.
- Pas de `CHANGELOG.md` : notes de version dans le tag annoté (convention org).

## Sécurité / confidentialité

Voir `SECURITY.md` hérité de l'org. Aucune PII réelle dans le code, les fixtures ou les logs —
particulièrement sensible ici puisque `kern-orch` orchestre des runs qui peuvent transporter
du contenu utilisateur.
