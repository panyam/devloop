# dagbump

You release goutils, and now servicekit, oneauth, mcpkit, chakra and jaala each need a new
version, in that order. dagbump works out that order and runs the mechanical steps.

```
make install-dagbump        # or: go install github.com/panyam/devloop/cmd/dagbump@latest
```

## The plan file

A cascade runs through one file, `dagbump.plan`:

```
dagbump plan mcpkit --write        # waves, each unit's commits and dependency updates
# fill in bump and notes for each release unit, by hand or through an agent:
dagbump notes --agent 'claude -p --permission-mode plan'
dagbump apply                      # check the file, print the table
dagbump apply --dry-run            # print every step
dagbump apply --yes                # update, release, push, publish, wave by wave
```

`plan --write` records each unit's wave, action, current version and the commit it was planned
against. It also records the unit's commits, changed folders and dependency updates. It leaves
`bump`, `version`, `notes`, `only` and `skip` empty for whoever writes the notes.

`notes --agent` runs a command once per release unit with empty notes, in that unit's repo
folder. Units in one wave run in parallel, up to `--jobs`. The command reads a prompt on stdin:
instructions, then the unit's context as JSON. `$DAGBUMP_CONTEXT` names a file holding the same
JSON. The command prints optional front matter (`bump: minor`) and then the notes. Claude in
plan mode can read the repo and run `git show`, and it cannot edit anything.

`apply` refuses a unit whose repo has commits the plan did not see, since its notes would
describe the wrong change set. Before each unit it reloads the graph, so each unit sees the tags
pushed by the units before it. After each unit it writes `status`, `released`, `tags` and
`error` back into the file. A rerun skips the units marked done.

Notes go into the repo as well. dagbump adds an entry at the top of `CHANGELOG.md` when the repo
has one. It writes `docs/releases/<V>.md` when the repo has a `docs/releases` folder. Both land
in the `release: V` commit. The notes also become the marker tag's annotation and the GitHub
release body. Set `notes: {changelog: ..., file: ...}` in `.dagbump.yaml` to choose other
paths, or `none` to turn one off.

## The manifest

The manifest lists the projects, one clone each. dagbump reads `--manifest`, else
`$DAGBUMP_MANIFEST`, else `~/.config/dagbump/dagbump.yml`. Keep it in a dotfiles repo and point
`$DAGBUMP_MANIFEST` at it. To keep separate lists, such as personal and work repos, point the
variable at another file on that machine or in that shell. `dagbump manifest` prints the file in
use and where its path came from. The manifest holds no edges and no release steps.
Every run reads each project's tracked go.mod and package.json files and builds the edges.

```yaml
owners: [panyam, turnforge]
roots: [~/newstack, ~/projects]
ignore_commits: ['^release: ']      # commit subjects that are not unreleased work
tools: {npm: pnpm}                  # default tool; enforce: true makes it the rule
projects:
  - name: mcpkit
    path: ~/newstack/mcpkit/main
  - name: blog
    path: ~/projects/blog/master
    kind: app                         # lib | app; default: lib when the repo has a semver tag
    depends_on: [s3gen]               # manual edge
    # skip: true leaves the project out
```

### Tools

Each npm package gets one tool, npm, pnpm or yarn, chosen in this order:

1. `tools: {npm: pnpm}` in the repo's `.dagbump.yaml`
2. `packageManager` in the package.json (the corepack pin)
3. the nearest lockfile, walking up to the repo root
4. `tools.npm` in the manifest
5. npm

dagbump warns when a declared tool disagrees with the lockfile. `update` installs with the
chosen tool, and a direct publish uses `pnpm publish` for pnpm packages, so `workspace:` specs
get rewritten. Setting a package's version is a plain edit of package.json, so no tool runs
for that step.

To make one tool the rule, set it in the manifest:

```yaml
tools:
  npm: pnpm
  enforce: true
```

With `enforce`, `update` refuses to install with another tool, and `release` refuses to
publish with one. The refusal names the fix (`pnpm import`, then commit `pnpm-lock.yaml`). A repo
opts out by naming its own tool in its `.dagbump.yaml`. `dagbump list --json` shows each
package's tool and where the choice came from.

`discover` scans each root's children and grandchildren for clones. It groups the clones by
remote and picks one per repo. A clone on the default branch wins, then a folder named main or
master, then a clean tree, then the newest commit. Existing entries keep their path.

## Per-repo config

Each repo can carry a `.dagbump.yaml` at its root. It says which Go modules and npm packages
share a version, and how a release runs. With the file present, `dagbump release` runs every
step itself, and the repo's `make tag`, `tag-push` and `bump-root` targets are no longer needed.
Without the file, dagbump falls back to those make targets.

`dagbump init <project>` proposes the file from the repo's tags. It puts the modules tagged at
the latest release, every public package.json, and `VERSION` into one lockstep unit. Review the
proposal, then add `--write`.

A repo with one version line puts the fields at the top level:

```yaml
# goapplib: Go module, two npm packages and VERSION share one version; CI publishes npm.
modules: [.]
npm: [tsappkit, tsappkit-solid]
version_files: [VERSION]
publish: {npm: ci}
```

```yaml
# mcpkit: 22 sub-modules tagged with the root; strip in-repo replaces at the tag.
modules: [., cmd/*, ext/*, stores/redis, experimental/ext/**, experimental/integrations/*,
          examples/mcpskills-walkthrough, tests/e2e, tests/keycloak]
npm: [experimental/ext/rooms/ts]
strip_replaces: true
publish: {npm: ci}
hooks:
  pre_tag: [make -s verify-submodule-deps]
```

A repo with several version lines lists them under `units`. The first unit is the main unit.
It carries the root tag and takes every module and package that no other unit claims. Another
unit is named `project/unit` in plans, tags only its own `<dir>/vX` tags, and waits in the plan
until the units it depends on have released.

```yaml
units:
  - modules: [., ext/*]
  - name: agent
    modules: [agent/**]
    versioning: changed
```

| Field | Meaning |
|---|---|
| `modules` | go.mod folders tagged with the unit. `.` is the root, `*` one segment, `**` any depth. Default: every module with a tag of its own |
| `follow` | Untagged go.mod folders (examples, docs) that belong to the unit. Default for the main unit: every module nobody claims |
| `pin_followers` | Which pins a follower moves at a release: `root` (default), `all` or `none`. Requires at `v0.0.0` never move |
| `npm` | package.json folders that get the unit's version. Default for the main unit: every public package.json |
| `version_files` | Files that hold the bare version. Default: `VERSION` if tracked. A file that had no `v` keeps none |
| `versioning` | `lockstep` (default) tags every module each release. `changed` tags the modules changed since their own last tag, plus the unit's modules that require them |
| `strip_replaces` | Tag a commit without in-repo replaces, then restore them, so `go install pkg@vX` works |
| `publish.npm` | `direct` (dagbump runs npm publish), `ci` (a workflow publishes on the tag) or `none`. Default: `ci` when a workflow mentions npm publish |
| `publish.github_release` | Default true |
| `notes.changelog`, `notes.file` | Where notes land. Default: `CHANGELOG.md` if present, `docs/releases/{V}.md` if that folder exists. `none` turns one off |
| `hooks.pre_tag`, `post_tag`, `post_release` | Shell commands run in the repo root with `V` and `NOTES` set |
| `check` | Runs after a dependency update in place of `go build ./...` |
| `tools.npm` | Top level only. This repo's npm tool: `npm`, `pnpm` or `yarn`. Overrides detection and the manifest's enforced choice |
| `kind`, `ignore_commits` | Top level only. Same meaning as in the manifest |

To release one module of a lockstep unit without the rest, run
`dagbump release mcpkit --version v0.9.1 --only ext/auth`. That call tags `ext/auth` and every
module of the unit that requires it. The unit's other modules keep their versions.

### Moving a repo off its make targets

1. Run `dagbump init <project>` and compare its module list with the Makefile's tag list.
2. Commit `.dagbump.yaml`, then run `dagbump release <project> --version vX --dry-run` and
   compare the steps with `make tag-push`.
3. Replace the release targets with one call:
   `release: ; dagbump release --version $(V) $(if $(NOTES),--notes $(NOTES))`.
4. Keep the scripts that CI still runs, such as goapplib's `check-versions.sh`, mcpkit's
   `rooms-ts-version.sh check`, and `release-replaces.sh restore` in vulncheck.yml.

## Commands

| Command | What it does |
|---|---|
| `discover [roots] [--owner o] [--write]` | Scan folders and merge new repos into the manifest |
| `init [project] [--write]` | Propose a `.dagbump.yaml` from the repo's tags and files |
| `add <path>` / `remove <name>` | Edit the manifest by hand |
| `link <dependent> <dependency>` / `unlink` | Add or drop a manual edge |
| `list` | Version, unreleased commits, lagging deps and warnings per project |
| `graph [--dot]` | The whole DAG in release waves |
| `plan <seeds> \| --changed [--exclude x] [--write]` | The seeds plus everything downstream, in waves, with an action each. A project name seeds all its units. `--write` saves `dagbump.plan` |
| `notes [plan] --agent cmd` | Fill bump and notes in the plan file through an agent |
| `apply [plan] [--dry-run \| --yes]` | Check the plan file, then run it wave by wave |
| `context <name>` | Commits, merges, diffstat and pending updates since the last tag, for writing notes |
| `update [name] [--push] [--wait 10m]` | Move deps on other units to their latest tags, tidy, build, commit. Waits for CI-published versions to appear |
| `release [name] --version vX \| --bump b [--notes f] [--only dirs]` | Tag, push, publish |

Every command takes `--json`, and `update` and `release` take `--dry-run`. `init`, `context`,
`update` and `release` default to the repo holding the current folder, so a Makefile can call
`dagbump release` without a manifest.

## How it decides

- A module is *published* when it has its own tag (`v1.2.3`, or `ext/auth/v1.2.3` for a
  sub-module). An edge *propagates* when the requiring module is the repo root or a published
  sub-module. A release of the dependency then forces a release of the dependent. Edges from
  examples and docs sites don't propagate, but `update` still moves them.
- A dependency *lags* when its required version is below the latest tag for that module.
  For npm, dagbump compares the locked version, or the floor of the spec. A `*` spec with no
  lock never lags. It finds the lockfile by walking up from the package to the repo root:
  `pnpm-lock.yaml` (v6 and v9 importers, and v5), `yarn.lock` (v1 and berry), or
  `package-lock.json`. The lockfile also picks the tool `update` runs: `pnpm add` / `pnpm update`,
  `yarn add` / `yarn upgrade`, or `npm install` / `npm update`.
- Before a pnpm install, `update` adds the new version to `minimumReleaseAgeExclude` in
  `pnpm-workspace.yaml`, when the file keeps that list. Without that entry pnpm refuses a version
  published minutes ago.
- `plan` reads live state. Rerun it after each release to see what is left.

## Release steps

`release` refuses when the tree is dirty, when the clone is off its default branch, when a
dependency lags, or when nothing is unreleased. With a `.dagbump.yaml` it runs these steps:

1. Write `V` to the version files and to each package.json in the unit.
2. Pin the tagged modules to `V` on each other, adding an in-repo replace where one is missing,
   so `go mod tidy` resolves `V` before it exists. Followers move per `pin_followers`. Tidy.
3. Run the `pre_tag` hooks and commit `release: V`.
4. With `strip_replaces`, commit the tree without in-repo replaces.
5. Tag. The marker tag goes first: the root tag for the main unit, and it carries the notes.
   Then a `<dir>/V` tag goes on each module.
6. Restore the replaces in a new commit, then run the `post_tag` hooks.
7. Push the branch, then the marker tag alone, then the rest in one push. GitHub raises no push
   event for a push carrying many tags, so the marker tag goes first on its own.
8. Run `npm publish` when `publish.npm` is direct, then `gh release create`, then the
   `post_release` hooks.

`update` and `release` set GOPRIVATE to the projects' module paths. `go mod tidy` then
resolves a tag pushed a moment ago, without waiting for proxy.golang.org. npm specs that start
with `link:`, `file:` or `workspace:` count as local, like a Go replace, so dagbump never
updates them.
