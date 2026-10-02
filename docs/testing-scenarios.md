# Tests de comportement sur cartes personnalisées

## But

Valider un comportement de la simulation (chasse, fuite, peur, territoire, sommeil,
décisions des bots…) avec un test Go court, sur une mini-carte écrite à la main, dont
la sortie tient en quelques lignes.

Règles :

- Un test = un comportement = une carte de 16×16 à 32×32 tuiles.
- Aucun `t.Logf` libre. En cas de succès, un test n'affiche rien.
- Les entités se désignent par un **label**, jamais par un `entity.ID`.
- Ne lance jamais une partie procédurale complète pour vérifier un comportement.
- Tout le code, les noms de fichiers et les commentaires restent en anglais
  (voir `Claude.md`).

Implémente les étapes dans l'ordre. Chaque étape se termine par
`go test ./sim/... -count=1` vert avant de passer à la suivante.

---

## Étape 1 — Grille rectangulaire

`sim/terrain/terrain.go` :

- `Grid` stocke `w, h` à la place de `size`.
- `NewGrid(w, h int)`, `Width()`, `Height()`. `InBounds`, `Tile`, `SetTile`,
  `Vertex` et `SetVertex` utilisent `w` et `h`. Il y a `(w+1)×(h+1)` sommets.
- Ajoute `NewGridSquare(n)` et `Size()`. `Size()` panique si `w != h` et ne sert
  qu'au code encore carré.

Propage le changement partout où `.Size()` est utilisé (`grep -rn "\.Size()" sim
wasm tools`) : `sim/core` (`occupied`, `inBounds`), `sim/fog` (`fog.New(w, h)`,
`PackedLen`), `sim/worldgen`, `sim/bots`, `wasm/main.go` et `tools/mapstat`. Côté
client : `client/src/sim/bridge.ts` et `client/src/render/terrainChunks.ts`.

`worldgen.Generate` produit toujours une carte carrée.

Taille autorisée : de 16 à 2048 par côté.

Le layout du hash change : incrémente `WorldVersion` et `hashLabel` dans
`sim/core/world.go`, puis réenregistre les replays de `tools/replays/` avec
`bin/detcheck -record`.

**Accepté quand** : `make test-sim test-det test-parity test-client` passe.

---

## Étape 2 — Format de carte `rts.map/custom/v1`

Une carte est un dossier :

```
wolf_hunts_rabbit/
  map.json
  terrain.png   (optionnel)
  height.png    (optionnel)
  biome.png     (optionnel)
```

`map.json` :

```json
{
  "format": "rts.map/custom/v1",
  "name": "wolf_hunts_rabbit",
  "width": 24, "height": 16,
  "origin": { "seed": 0 },
  "terrain": { "rows": [
    "........................",
    "....FFFF................",
    "...........~~~~........."
  ] },
  "height": { "fill": 0 },
  "biome":  { "fill": "temperate" },
  "players": [
    { "spawn": [3, 3], "era": "prehistory", "startKit": "none",
      "resources": { "food": 0, "wood": 0, "stone": 0, "gold": 0, "oil": 0 } }
  ],
  "nodes":     [ { "kind": "wood", "tree": "birch", "x": 5, "y": 1, "amount": 100 } ],
  "units":     [ { "label": "wolf",   "def": "wolf",   "owner": "neutral", "x": 18, "y": 8 },
                 { "label": "rabbit", "def": "rabbit", "owner": "neutral", "x": 12, "y": 8 } ],
  "buildings": [ { "label": "fire", "def": "campfire", "owner": 0, "x": 6, "y": 6, "complete": true } ],
  "relics":    []
}
```

Couches `terrain`, `height` et `biome` : chacune contient **exactement une** des
formes suivantes.

| Forme | terrain | height | biome |
|---|---|---|---|
| `"fill": v` | clé de terrain (`"grass"`) | entier 0..64 | clé de biome |
| `"rows": [...]` | `h` chaînes de `w` caractères | `h+1` chaînes de `w+1` entiers séparés par des espaces | `h` chaînes de `w` caractères (ids 0..9) |
| `"png": "file"` | PNG 8 bits `w×h`, pixel = `terrain.Type` | PNG 8 bits `(w+1)×(h+1)`, pixel = hauteur | PNG 8 bits `w×h`, pixel = id de biome |

Légende ASCII du terrain : `.` grass, `F` forest, `R` rock, `s` sand, `~` shallow,
`W` deep.

Autres règles :

- `def`, `tree`, `kind` et `era` sont les **clés** de `data/*.json` (`"key"`), jamais
  des ids numériques. `owner` vaut un index de joueur ou `"neutral"`.
- `startKit` vaut `"default"` (entrepôt + 3 villageois, comme aujourd'hui dans
  `initEconomy`) ou `"none"`.
- `origin.seed` est une métadonnée. **Ne relance jamais la génération au chargement.**
- `relics` absent : placement automatique (`placeRelics`). Liste vide : aucune relique.

### Paquet `sim/mapfile`

```go
type Scenario struct {
    Name      string
    Map       *worldgen.Map
    Players   []PlayerStart   // Spawn, Era, StartKit, Res [resource.Count]int32
    Units     []UnitPlacement // Label, DefID, Owner, X, Y
    Buildings []BuildingPlacement // Label, DefID, Owner, X, Y, Complete
}

func Load(dir string) (*Scenario, error)
func Parse(mapJSON []byte, files map[string][]byte) (*Scenario, error)
func Save(dir string, s *Scenario) error // rows up to 64x64, png above
```

- Ajoute `worldgen.FromLayers(grid, biomes, spawns, nodes, animals, relics) *Map`
  à côté des `Custom*` de `sim/worldgen/worldgen.go`. Le digest (`hashMap`) est
  calculé sur les données décodées.
- Les erreurs citent le champ et la valeur : `units[2].x=40 out of bounds (width 24)`,
  `terrain.rows[3]: len 23, want 24`, `units[0].def "wolff": unknown unit key`.
- Le PNG est décodé avec `image/png`, uniquement au chargement.

**Accepté quand** :

- Pour une même carte, un aller-retour `Save`/`Load` en `rows` et en `png` donne le
  même `Map.Hash()`.
- Un test de table couvre chaque erreur de validation.

---

## Étape 3 — Mise en place exportée (`sim/core/setup.go`)

Ces fonctions ne sont jamais atteignables par un ordre, comme `DebugSpawnUnits`.

```go
type Labels struct{ /* name -> ids, slices only, no Go map iteration */ }
func (l Labels) Unit(name string) entity.ID
func (l Labels) Building(name string) entity.ID

func NewWorldFromScenario(seed uint64, s *mapfile.Scenario) (*World, Labels)
func (w *World) SpawnUnit(owner uint8, defID uint16, tx, ty int32) entity.ID
func (w *World) PlaceBuilding(owner uint8, defID uint16, tx, ty int32, complete bool) entity.ID
func (w *World) SetEra(player uint8, era uint8)
func (w *World) SetResources(player uint8, res [resource.Count]int32)
```

- Sors de `initEconomy` la partie « entrepôt + villageois » dans
  `applyStartKit(player)`. Elle n'est appelée que pour `startKit == "default"`.
  `NewWorld` et `NewWorldSize` gardent exactement le comportement actuel.
- Remplace les helpers de test privés `spawnUnitAt` (`sim/core/combat_test.go`) et
  `placeBuilding` (`sim/core/siege_test.go`) par ces fonctions.

**Accepté quand** : les replays existants restent inchangés, soit
`make test-det` vert sans réenregistrement après l'étape 1.

---

## Étape 4 — Modules du bot

`sim/bots/bots.go` :

```go
type Module uint16
const (
    ModEconomy Module = 1 << iota
    ModBuild
    ModAgeUp
    ModMilitary
    ModResearch
    ModWaves
    ModRelics
    ModExplore
    ModAll = ModEconomy | ModBuild | ModAgeUp | ModMilitary | ModResearch | ModWaves | ModRelics | ModExplore
)
func (b *Brain) Only(m Module) *Brain
```

`New` utilise `ModAll`. `Brain.Step` n'appelle que les modules actifs.

**Accepté quand** : `TestDeterminismOfBrains` et `TestBotReachesAntiquity` passent
sans modification.

---

## Étape 5 — Kit de test `sim/simtest`

Déplace `drive` de `sim/bots/bots_test.go` dans ce paquet et exporte-la.

```go
func Load(t *testing.T, dir string) *S           // mapfile.Load + NewWorldFromScenario
func (s *S) Bots(b ...*bots.Brain) *S
func (s *S) U(label string) UnitView               // State(), Tile(), HP(), Target(), Dead(), DistHome(), DistTo(UnitView)
func (s *S) B(label string) BuildingView           // HP(), Dead()
func (s *S) World() *core.World

func (s *S) Within(ticks uint32) Step              // steps until cond true, fails at deadline
func (s *S) During(ticks uint32) Step              // steps the whole window
func (st Step) Expect(cond func() bool, what string)
func (st Step) Never(cond func() bool, what string)
func (s *S) ApproxEq(got, want, tol int32, what string)
func (s *S) Snapshot(name string)                  // testdata/golden/<name>.txt, -update regenerates
```

### Sortie en cas d'échec : 15 lignes maximum

À chaque tick, `S` enregistre les entités étiquetées dans un tampon circulaire de
64 échantillons (tick, état, tuile, PV, label de la cible). En cas d'échec, il
affiche les 8 derniers échantillons :

```
FAIL rabbit flees wolf: not true within 200 ticks (t=0..200)
  t=186 rabbit wander (12,8) hp=10 tgt=-      | wolf attack (14,8) hp=40 tgt=rabbit
  t=193 rabbit wander (12,8) hp=10 tgt=-      | wolf attack (13,8) hp=40 tgt=rabbit
  t=200 rabbit dead                           | wolf idle   (13,8) hp=40 tgt=-
```

- Utilise `t.Helper()` et `t.Fatalf`. Rien n'est affiché en cas de succès.
- Avec `RTS_TRACE=1`, `S` écrit l'historique complet des entités étiquetées dans
  `build/trace/<TestName>.txt`. Rien ne va sur stdout.
- Le snapshot est un texte trié : les joueurs (ère, ressources), puis chaque entité
  étiquetée (état, tuile, PV).

**Accepté quand** : les tests de `sim/simtest` passent, et un test volontairement
faux produit un message de 15 lignes maximum.

---

## Étape 6 — Tests de comportement

Emplacement : les cartes vont dans `sim/core/testdata/maps/<name>/`, les tests dans
`sim/core/behavior_test.go`, avec des noms de la forme `TestBehavior<Nom>`.

Exemple :

```go
func TestBehaviorWolfHuntsRabbit(t *testing.T) {
    s := simtest.Load(t, "testdata/maps/wolf_hunts_rabbit")
    wolf, rabbit := s.U("wolf"), s.U("rabbit")
    s.Within(100).Expect(func() bool { return rabbit.State() == core.UnitFlee }, "rabbit flees wolf")
    s.Within(600).Expect(rabbit.Dead, "wolf kills rabbit")
}
```

Premiers tests à écrire : une carte par test.

| Test | Vérifie |
|---|---|
| `WolfHuntsRabbit` | le loup chasse et tue le lapin |
| `SheepFleesWolf` | le mouton passe en `UnitFlee` quand le loup approche |
| `BearStaysInTerritory` | `Never(DistHome > leash)` sur 2000 ticks |
| `AnimalsSleepAtNight` | `UnitSleep` pendant la nuit (`wildSleep`) |
| `CampfireShelters` | les animaux n'entrent pas dans le rayon d'un feu de camp (`nearCampfire`) |
| `PackAvoidsGuardedVillager` | les loups n'attaquent pas un villageois gardé (`guardsNear`) |
| `BotEconomyGathers` (`sim/bots`) | avec `Only(ModEconomy)`, les réserves de bois augmentent |
| `BotWavesAttackVisibleEnemy` (`sim/bots`) | avec `Only(ModWaves)`, l'armée cible un ennemi visible |

Pour les comportements (états, cibles, territoire) : assertions exactes.
Pour les combats (PV, durée d'un duel) : `ApproxEq` avec une tolérance.

---

## Étape 7 — Outils

- `tools/mapcheck` :
  - `mapcheck <dir>` affiche une ligne (`ok 24x16 players=1 units=2 buildings=1 digest=...`)
    ou l'erreur de validation. Code de sortie 1 en cas d'erreur.
  - `mapcheck -ascii <dir>` affiche le terrain en ASCII, avec les unités par
    l'initiale de leur label.
- `Makefile` : ajoute `test-behavior` : `go test ./sim/... -run Behavior -count=1`.
  `make test` inclut déjà ces tests via `test-sim`.

---

## Boucle de travail

1. Écris la carte (`rows`), puis vérifie-la avec `mapcheck -ascii`.
2. Écris le test avec des labels.
3. Lance `make test-behavior`.
4. Lis seulement le message d'échec. S'il ne suffit pas, relance avec
   `RTS_TRACE=1` et ouvre le fichier de `build/trace/`.
