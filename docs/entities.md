# Règles des entités : vision, déplacement, faim, proies, états

## But

Rendre le comportement des entités (animaux et humains) piloté par ce qu'elles voient,
leur faim et leurs états. Les règles se définissent par espèce dans `data/units.json`.

Règles :

- Tout le code, les noms de fichiers et les commentaires restent en anglais
  (voir `Claude.md`).
- La simulation reste déterministe : entiers et `fixed.F16` uniquement, jamais de `float`.
- Chaque comportement nouveau a un test de comportement sur mini-carte
  (voir `docs/testing-scenarios.md`, cartes dans `sim/core/testdata/maps/`).
- Chaque étape qui change l'état hashé incrémente `hashLabel` (`sim/core/world.go`,
  actuellement `rts.world/v16`) et réenregistre les replays de `tools/replays/`.

Implémente les étapes dans l'ordre. Chaque étape se termine par
`go test ./sim/... -count=1` vert avant de passer à la suivante.

---

## État actuel

- `Unit.X/Y` sont déjà en `fixed.F16`, mais `moveUnit` et `followField`
  (`sim/core/economy.go`) avancent de centre de case en centre de case via
  `stepToward(tileCenter(nx), tileCenter(ny), ...)`. Le déplacement est calé sur la grille.
- La sim n'a pas de cap. Le client calcule l'orientation avec `Math.atan2(st.vx, st.vy)`
  (`client/src/render/actors.ts`).
- Le brouillard (`updateVision`, `sim/core/economy.go`) applique `fog.ApplyCircle`
  pour chaque unité (`visionOf`) et chaque bâtiment (`BuildingVision` 8, `SiteVision` 4).
- La faune (`sim/core/wildlife.go`) détecte en cercle : `flee`, `fleeRadius`,
  `fleeClear`, `aggro`, `prey` dans `data/units.json` (`sim/units/units.go`).
- Le blob unités de `wasm/main.go` (`unitsBlob`) fait 24 octets par unité, les octets
  22-23 sont libres.

---

## Étape 1 - Cap et déplacement libre

`sim/core/entities.go` :

- Ajoute `Unit.Heading int32`, en degrés entiers, 0-359. 0 pointe vers +Y, sens horaire.
- Ajoute des tables sin/cos entières sur 360 degrés dans un petit fichier
  `sim/core/angle.go` (valeurs `fixed.F16`, générées une fois, codées en dur), et
  `headingTo(dx, dy fixed.F16) int32` (atan2 entier par octants + table).

`sim/units/units.go` et `data/units.json` :

- Nouveau champ `turnRate` (degrés par tick). Valeur par défaut si absent : 30.

Déplacement (`moveUnit`, `followField`, `stepToward`) :

- Le flow field donne une direction : le centre de la case suivante sert uniquement
  à calculer le cap voulu.
- L'unité tourne vers ce cap d'au plus `turnRate` par tick, puis avance de sa vitesse
  selon son `Heading`. Elle ne se recale plus sur le centre des cases.
- Une unité ne peut pas entrer dans une case non marchable : si le pas y mène, elle
  glisse le long de l'axe libre ou reste sur place.
- L'arrivée sur la cible exacte (`TX/TY`) reste un `near(...)` puis un recalage sur la
  cible.
- Une unité qui attaque, récolte ou construit se tourne vers sa cible.

Snapshot et client :

- `unitsBlob` écrit `Heading` en `uint16` à l'offset 22.
- `client/src/sim/bridge.ts` le lit. `actors.ts` utilise ce cap à la place de
  `atan2` et de `facingTarget`, avec le lissage `stepYaw` existant.

Tests :

- `TestFreeMoveNotGridAligned` : une unité envoyée en diagonale sur 10 cases passe par
  des positions qui ne sont pas des centres de case.
- `TestTurnRateLimit` : une unité qui fait demi-tour met `180 / turnRate` ticks.

---

## Étape 2 - Champ de vision paramétrable

`data/units.json` (champs par espèce) :

- `vision` (existant) reste la portée en cases.
- Nouveau `visionArc` : ouverture en degrés, centrée sur `Heading`. 360 = cercle.
  - Proies : arc large.
  - Prédateurs : cône.
  - Humains : à définir par unité.
- Validation dans `sim/units/units.go` : `visionArc` entre 1 et 360, 360 par défaut.

`sim/core/vision.go` (nouveau) :

- `func (w *World) sees(u *Unit, x, y fixed.F16) bool` : distance au carré
  <= `vision²` ET angle entre `Heading` et la direction de la cible <= `visionArc / 2`.
- Une unité qui dort ne voit rien.

Remplace toutes les détections circulaires par `sees` :

- `nearestThreat`, `nearestThreatID` (`combat.go`, `wildlife.go`).
- `wildAcquire`, `wildWoken`, `wildDetect`, le calcul de fuite (`wildlife.go`).
- L'acquisition de cible en combat (`combatPhase`, `combat.go`).

Tests :

- `TestConeSeesAhead` : un loup voit un lapin devant lui et ne voit pas un lapin
  derrière lui à la même distance.
- `TestWideArcSeesBehind` : un mouton voit un loup placé dans son dos.

---

## Étape 3 - Brouillard

`sim/fog/fog.go` :

- Ajoute `ApplyCone(cx, cy, r, heading, arc int)` : même règle que `sees`, par case.
  `ApplyCircle` reste pour les feux de camp.

`updateVision` (`sim/core/economy.go`) :

- Chaque unité active applique `ApplyCone` avec son `Heading` et son `visionArc`.
- Seuls les feux de camp terminés révèlent une zone : cercle de 10 cases
  (nouvelle constante `CampfireVision = 10`).
- Les autres bâtiments et les chantiers ne révèlent plus rien. Supprime
  `BuildingVision` et `SiteVision`.
- L'état « exploré » ne change pas : une case vue reste explorée.

Tests :

- `TestFogConeOnly` : une case derrière un villageois (hors de son arc) reste non
  visible.
- `TestFogCampfireRadius` : les cases à 10 cases d'un feu de camp sont visibles, celles
  à 11 ne le sont pas.
- `TestFogBuildingsDark` : une caserne seule ne révèle rien.

---

## Étape 4 - Altérations d'état

`sim/core/entities.go` :

- Ajoute `Unit.Cond uint8` (bitmask) et `Unit.CondTicks int32` (minuteur).
- Bits : `CondFear = 1`, `CondTerror = 2`, `CondRage = 4` (Rage : étape 5).

Règles :

- **Peur** : une entité ennemie est dans le champ de vision (`sees`). Effet :
  évitement. L'entité s'éloigne de la menace en gardant son activité si possible.
  La Peur disparaît quand plus aucune menace n'est vue.
- **Terreur** : l'entité subit des dégâts. Effet : fuite directe à l'opposé de
  l'attaquant, pendant un nombre de ticks fixé dans `data/units.json`
  (`terrorTicks`).
- La Terreur est prioritaire sur la Peur.

Qui ressent quoi :

- Les proies ressentent Peur et Terreur face aux entités hostiles.
- Parmi les humains, seules les unités non combattantes (villageois) ressentent Peur
  et Terreur, et seulement à la vue d'un prédateur (espèce avec une liste de proies,
  voir étape 6).
- Les unités militaires ne ressentent ni Peur ni Terreur.

Remplace `flee`, `fleeRadius` et `fleeClear` par ces états. Retire ces champs de
`units.json` et de `sim/units/units.go`.

Tests :

- `TestFearAvoidsSeenWolf` : un mouton qui voit un loup s'éloigne. Le même mouton ne
  réagit pas si le loup est hors de son champ de vision.
- `TestTerrorAfterDamage` : un cerf touché fuit à l'opposé de l'attaquant.
- `TestVillagerFearsPredator` : un villageois qui voit un loup a `CondFear`.
- `TestSoldierNoFear` : un lancier qui voit un loup n'a aucun état.

---

## Étape 5 - Faim des prédateurs

Seules les espèces avec une liste de proies (étape 6) ont une faim.

`sim/core/entities.go` et `data/units.json` :

- Ajoute `Unit.Hunger int32`, de 100 (rassasié) à 0 (affamé).
- `hungerDecay` : ticks entre deux points de faim perdus.
- Manger une carcasse remonte `Hunger` de la valeur `corpseFood` de la proie, plafonné
  à 100. Branche dans `processDeaths` (`combat.go`) quand le tueur est un prédateur.

Seuils :

- **20 %** : le prédateur arrête ses comportements d'évitement (aura de feu de camp,
  proies escortées via `guardsNear` / `PreyEscortMax`).
- **10 %** : Rage (`CondRage`). Plus rapide et plus agressif. Les multiplicateurs sont
  dans `units.json` (`rageSpeed`, `rageAggro`).
- **0 %** : perte de PV à intervalle fixe jusqu'à ce qu'il mange ou meure.

Tests :

- `TestHungerDecays` : la faim d'un loup baisse de 1 tous les `hungerDecay` ticks.
- `TestEatingRefills` : un loup qui tue un lapin gagne `corpseFood` de faim.
- `TestHungryIgnoresCampfire` : à 20 %, un loup entre dans l'aura d'un feu de camp.
- `TestRageAt10` : à 10 %, le loup a `CondRage` et une vitesse plus haute.
- `TestStarveAt0` : à 0 %, le loup perd des PV.

---

## Étape 6 - Listes de proies

`data/units.json` et `sim/units/units.go` :

- `preyPreferred` : proies chassées dès qu'elles sont vues.
- `preyFallback` : proies éventuelles, chassées seulement quand la faim est basse.
- `preyFallbackAt` : seuil de faim (%) qui active `preyFallback`. Valeur à fixer.
- Supprime `prey`. `isPrey` (`wildlife.go`) prend en compte la faim.

`wildAcquire` :

- Cherche d'abord la proie préférée vue la plus proche, puis une proie éventuelle si
  `Hunger <= preyFallbackAt`.

Tests :

- `TestPreferredPrey` : un loup rassasié à portée d'un lapin (préféré) et d'un mouton
  (éventuel) chasse le lapin.
- `TestFallbackOnlyWhenHungry` : un loup rassasié ignore un mouton seul. Sous
  `preyFallbackAt`, il le chasse.

---

## Étape 7 - Emoji d'état (client)

Snapshot :

- Passe `unitsBlob` à 28 octets par unité et écrit `Cond` à l'offset 24.
- Mets à jour la taille et la lecture dans `client/src/sim/bridge.ts`.

`client/src/render/actors.ts` :

- Affiche un petit sprite emoji au-dessus de l'entité quand `Cond` est non nul.
- Un emoji par état : Peur, Terreur, Rage. Si plusieurs états sont actifs, affiche le
  plus prioritaire (Terreur > Rage > Peur).

Vérification : lancer le client, faire approcher un loup d'un villageois et vérifier
l'emoji de Peur.

---

## Fin

- `go test ./sim/... -count=1` vert.
- `hashLabel` incrémenté et replays de `tools/replays/` réenregistrés.
- Mettre à jour `data/README.md` avec les nouveaux champs de `units.json`.
