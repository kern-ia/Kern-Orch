---
id: 0015
feature: async-step-reporter
branch: feature/async-step-reporter
status: done
files:
  - internal/report/http.go
  - internal/report/activity.go
  - internal/cmd/commands.go
tests:
  - internal/report/queue_test.go
  - internal/cmd/publish_skills_test.go
decisions:
  - "2026-07-28 : UNE file et UN worker, jamais une goroutine par événement — un sink replie les niveaux en séquence et rejette un niveau plus ancien, deux envois concurrents perdraient une frontière en silence"
  - "2026-07-28 : l'échec passe par la MÊME file que les niveaux — sinon il peut les doubler, et un run échoué semblerait revenir à la vie"
  - "2026-07-28 : file pleine = on jette, jamais on bloque — chaque événement porte l'état complet, le suivant corrige ce qui manque ; même arbitrage que le hub SSE côté interface"
  - "2026-07-28 : `Flush()` BORNÉ (3 s) — sinon on ne déplace le problème que du moteur vers la sortie du processus"
  - "2026-07-28 : `ActivityReporter.Flush()` borné aussi, il est sur le même chemin de sortie"
---

**Quoi** : le moteur ne s'arrête plus pour laisser l'interface le regarder. Mesuré sur un
puits à 3 s/requête et quatre niveaux : 8,4 s avant, 3,4 s après (l'abandon borné), et
0,014 s sans puits — le graphe lui-même n'attend plus rien.

**Pièges** :
- Rendre l'envoi asynchrone a cassé neuf tests existants qui affirmaient juste après l'appel.
  C'est la bonne conséquence : `Flush()` fait désormais partie du contrat.
- Premier jet : le graphe était libéré mais le processus attendait 8,4 s à la sortie. Un
  déplacement de problème n'est pas une correction — mesurer les DEUX bouts.
