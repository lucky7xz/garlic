<table><tr>
<td><img src="./logo.png" alt="logo" width="360"/></td>
<td><img src="./scnshot.png" alt="screenshot" width="480"/></td>
</tr></table>

# Garlic

*A chronyx.xyz project*

A terminal Kanban board built on your filesystem. Projects are markdown files, a project's status is a tag inside the file, and the board is what those files look like arranged into columns. Edit one in any editor and the board moves — there is no database, no index, nothing to keep in sync.

Built on the [PARA Method](https://fortelabs.com/blog/para/) (Projects, Areas, Resources, Archives) and re-imagined for bash-native workflows with a bring-your-own-tools mindset: garlic handles the *where*, your editor and file manager handle the *how*. It can also hand a project to another machine — often an agent — and collect the work later; see [Planting and harvesting](#-planting-and-harvesting).

## 🚀 Install

```bash
curl -fsSL https://raw.githubusercontent.com/lucky7xz/garlic/main/install.sh | sh
```

No toolchain needed: the installer picks the archive for your machine, verifies its checksum against the release, and installs to `/usr/local/bin` when that is writable and `~/.local/bin` otherwise — telling you if that directory is not on your `PATH`.

| | |
|---|---|
| **Runs on** | Linux, macOS, FreeBSD — x86_64, arm64, armv7 |
| **By hand** | static binaries, `.deb` and `.rpm` on the [Releases](https://github.com/lucky7xz/garlic/releases) page |
| **With Go** | `go install github.com/lucky7xz/garlic@latest` — lands in `~/go/bin`, which your shell may not search yet |
| **Stale version?** | `GOPROXY=direct go install github.com/lucky7xz/garlic/cmd/garlic@latest` |
| **Update** | rerun whichever command you installed with |
| **Which build is this?** | `garlic version` |

After `go install`, if your shell cannot find `garlic`: `echo 'export PATH=$PATH:~/go/bin' >> ~/.bashrc` (or `~/.zshrc`), then open a new shell.

### 🧄 Quick start

```bash
garlic init
```

Writes an example workspace to `~/shara` — projects, statuses, a resource folder — matching the default config, so there is a board to look at before you point garlic at your own files.

## How it works

Garlic scans the **first-level sub-directories** of each configured path and tracks the markdown files it finds there. Everything it knows comes out of the files themselves:

| in the file | means |
|---|---|
| `#statustag-inProgress` | the project sits in the `inProgress` row — any status you configure |
| `#garlic-hide` | it moves to the hidden view, toggled with `tab` |

```
~/shara/
├── epics/                        ← Full Bulb (every .md is tracked)
│   ├── fitness/
│   │   ├── running.md             ← (contains #statustag-inProgress)
│   │   └── running/              ← resource folder (marked 📎 when it holds something)
│   │       ├── plan.pdf
│   │       └── progress.csv
│   └── learning/
│       └── golang.md
│
├── scripts/                      ← Semi Bulb (only .clove.md tracked)
│   ├── garlic/
│   │   ├── revise.clove.md        ← tracked
│   │   ├── release.clove.md       ← tracked
│   │   └── main.go
│   └── neofetch/                 ← no .clove.md → invisible to Garlic
│       └── neofetch.sh
```

Each configured `path` is a workspace (a **bulb**), each status tag a horizontal section, each first-level sub-directory a column. There are two kinds of bulb, and the difference is what counts as a project:

| | full bulb | semi bulb |
|---|---|---|
| **tracks** | every first-level `.md` | only `.clove.md` files |
| **a project is** | the file, plus its same-named resource folder | the folder itself — the clove is a note about it |
| **a column is** | an area holding several projects | one project |
| **made for** | homogeneous collections: epics, decks, notes | source directories, where most of what is there is noise |

Two marks appear on the board:

| mark | means |
|---|---|
| 📎 | the project's resource folder holds something — an empty one is not marked |
| 🌱 | it is planted on a remote — see [Planting and harvesting](#-planting-and-harvesting) |

> [!TIP]
> Garlic watches your filesystem. Edits made outside the TUI show up in it instantly.

### Why plain text

| | |
|---|---|
| **No database** | a file is a file — nothing to corrupt, migrate or export |
| **Any tool works** | any editor, `grep`, `git` for backup, whatever you sync with |
| **Encryption-friendly** | plain text pairs naturally with `gpg`, and is easy to audit |

## ⌨️ Keys

`?` brings up a quick reference inside the TUI.

**Navigate**

| key | |
|---|---|
| `h j k l`, arrows, `wasd` | move the cursor |
| `o` / `p` | previous / next workspace |
| `alt+1` … `alt+9` | jump to a workspace by number — the order garlic loads bulbs in, which is the `[2/3]` counter in the header |
| `tab` | show hidden projects |
| `q` | quit |

**Open** — garlic hands off to your own tools

| key | |
|---|---|
| `enter` / `space` | open the file in your editor |
| `alt+enter` | …in your alternative editor |
| `r` | open the resource folder in your file manager |
| `alt+r` | …in your alternative file manager |
| `alt+g` | a shell in the project's folder: here, or on the remote holding it |

**Manage**

| key | |
|---|---|
| `i` | new project file in this column |
| `m` | move it to another status |
| `e` | rename it |
| `u` | hide / unhide |
| `Del` | delete it, after typing the word |
| `g` | check what is planted on your remotes |

## ⚙️ Configuration

`~/.config/garlic/config.toml`. Primary and alternative tools give you two workflows on one key — `micro` on `enter` and `glow -p` on `alt+enter`, `yazi` on `r` and `dolphin` on `alt+r`. Commands take up to one flag.

| key | |
|---|---|
| `editor` / `alt_editor` | opens a project file. Unset falls back to `$EDITOR`, then `xdg-open` (`open` on macOS) |
| `file_manager` / `alt_file_manager` | opens a resource folder. Unset falls back to `$FILEMANAGER` |
| `alt_modifier` | the modifier for the alternatives, default `alt`. Workspace jumping stays on `alt+1..9` whatever you set, since terminals cannot send `ctrl+<digit>` as a distinct key |
| `async_apps` | launched detached, for GUI tools. Never put a TUI here — it would lose the terminal |
| `theme` | see [Theming](#-theming) |
| `[[full-bulb]]` / `[[semi-bulb]]` | a workspace: its `path`, its `statuses`, and optionally what to `ignore` |
| `[[remote]]` | a machine to plant to — see [Planting and harvesting](#-planting-and-harvesting) |

```toml
theme = "dracula"

editor           = "micro"
file_manager     = "yazi"
alt_editor       = "glow -p"
alt_file_manager = "dolphin"
alt_modifier     = "alt"

async_apps = ["xdg-open", "open", "dolphin", "gedit", "code"]

[[full-bulb]]
path     = "~/shara/epics"
statuses = ["inProgress", "onHold", "toDo"]

[[semi-bulb]]
path     = "~/shara/scripts"
statuses = ["inProgress", "onHold"]
ignore   = ["dist", "node_modules"]
```

> [!IMPORTANT]
> Upgrading from before `async_apps`, `alt_editor` and `alt_file_manager` existed? Copy them across from the [default template](internal/config/bootstrap/config.toml). A `[[remote]]` block must stay **last** in the file — a TOML table header captures every key that follows it.

## 🌱 Planting and harvesting

Hand a project to another machine — often an agent — let it work, and collect the
results. This machine is always the source of truth; the remote is derived and
disposable.

**1. Point garlic at the machine** in `~/.config/garlic/config.toml`:

```toml
[[remote]]
name          = "agent"
host          = "you@1.2.3.4"
root          = "~/shara"             # a path on the OTHER machine
# port          = 2222
# identity_file = "~/.ssh/agent_key"
```

Needs `ssh`, `rsync` and `sha256sum` at both ends. Nothing is installed over
there — garlic writes a plain tree inside `root` and nothing outside it.

**2. Use the four verbs:**

| | |
|---|---|
| `garlic plant epics/fitness/running @ agent` | send it |
| `garlic status @ agent` | what changed over there? |
| `garlic harvest epics/fitness/running @ agent` | bring the work back |
| `garlic wipe epics/fitness/running @ agent` | reset it, then plant again |

The address is the board's own coordinates — `bulb`, `bulb/area`, or
`bulb/area/project` — so `garlic plant epics` sends a whole bulb. `@ <remote>` is
always required, which is why `garlic plant epics` alone can never reach another
machine. Tab completion fills addresses in; `garlic status` offers to set it up
the first time it finds it missing, or `garlic completion bash` prints the hook
to place yourself.

**What travels:** on a full bulb, a project file plus its resource folder. On a
semi bulb the folder *is* the project, so all of it goes. Projects you have
hidden with `#garlic-hide` stay home — that is what hiding means. `.git` stays
home too unless you ask for it (below), and anything else you name in the bulb's
`ignore` list stays with it.

### 🔀 Handing over a git repo

`garlic plant scripts/garlic --git @ agent` sends the repository too, and checks
out a branch garlic owns on the remote — `garlic/agent`, named after the remote.
The agent commits there, against your real history.

Harvest then does not copy files, because that would flatten every commit into
one. It fetches:

```
garlic harvest scripts/garlic @ agent
  a git repository — commits travel, files do not

  fetched garlic/agent → refs/remotes/agent/garlic

  4 commits
    a3f9c1  add retry to the fetch loop
    7c2104  cover the timeout path

  nothing merged, nothing of yours touched.
    git merge refs/remotes/agent/garlic
```

Not merging is the point — it is the same stance harvest takes everywhere:
carry it across, leave the decision. Anything the agent left *uncommitted* is
named too, since no fetch can see it.

`--git` seeds the repo once and refuses afterwards: re-sending `.git` would push
your refs over the agent's. From then on git is the channel.

### 🌱 What is planted where

Press `g` on the board. Garlic asks every configured remote what it is holding
and marks what is out there. Columns are areas, cells are the projects in them:

| fitness | learning 🌱 |
|---|---|
| running 📎 🌱 | golang |
| swimming | |

`learning` went whole, so the **column** carries the mark and the project inside
it stays bare. In `fitness` only `running` was sent, so the **project** carries
it. The mark always shows what you sent, at the size you sent it.

Put the cursor on a marked project and the footer names the machine:

```
🌱 planted on agent 3d ago • checked 14:20   ← cursor on running
?: help • q: quit • 🌱 checked 14:20         ← cursor anywhere else
```

The two times answer different questions — how long the work has been over there,
and how stale this picture of it is.

Nothing is cached. An unmarked board means you have not asked yet, which is why
the footer stamps the time once you have.

<details>
<summary><b>How garlic knows who changed a file</b></summary>

Planting leaves a manifest on the remote: the hashes of what was handed over.
That gives garlic three states for every file — as planted, as it is on the
remote now, as it is here now — which is exactly what it takes to know *who moved
a file*, rather than merely that it differs.

- **Collected:** the agent changed it and you did not.
- **Parked:** you both changed it. Your file is untouched; the remote's version is
  set down inside the project's resource folder as `<name>.remote.md`. Parking
  counts as handing the conflict over, so the next `plant` will send your version
  — merge first if you want theirs.
- **Reported only:** the agent deleted it. **Garlic never deletes anything of
  yours** — `#garlic-hide` is the delete that travels, because it is content.
- **Left alone:** planting skips anything the agent has touched, the exact mirror
  of the rule above.
- **Left on the remote:** anything in an area you never planted into. Harvest
  collects only what the manifest covers, so a remote's own folders stay its own
  — but inside an area you did plant, whatever the agent adds comes home.

Garlic keeps no state on this machine. The manifest lives on the remote next to
what it describes — one per bulb, inside it — so wiping a bulb takes its record
along with its folder, and `harvest` against a remote with no manifest refuses
rather than guessing.

</details>

<details>
<summary><b>What never travels, and why</b></summary>

`.git` is never *collected*, and that is a correctness rule rather than a size
one: rsync merges without deleting, so a harvested `refs/` or `index` would land
on top of yours while your objects stayed, leaving branch pointers and worktree
disagreeing. Planting into an empty tree has no such hazard, which is why
`plant --git` exists and harvest has no counterpart — it fetches instead.

Everything else is up to you, via `ignore` on the bulb. Patterns match whole path
segments rather than substrings, so `dist` cannot swallow `distributed`.

</details>

<details>
<summary><b>Why wipe asks harder the wider it reaches</b></summary>

An address means the folder dies whole — including work the agent left that you
never harvested. No address means every bulb. Anything in the root that is not a
bulb is never touched, so a `root` shared with the remote's own work survives.

Because typing `epics` when you meant `epics/fitness` is the accident worth
preventing, wipe asks in proportion to what it would take: a project wants its
name typed, an area a confirmation and its name, a bulb the name and the file
count, the whole remote four answers. The count comes from the summary printed
first, so it cannot be typed without reading what is about to go.

</details>

## 🎨 Theming

Set your preferred theme in `~/.config/garlic/config.toml`:

```toml
theme = "dracula" # other options: dracula2, jade, nord, everforest, orasaka
```

> [!NOTE]
> If you use **[Drako](https://github.com/lucky7xz/drako)**, its theme settings will automatically override Garlic's theme.

---
*Built with [Charm](https://charm.sh/) 🧄*
