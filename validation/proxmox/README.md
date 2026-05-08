# Proxmox validation orchestrator

Bash scripts that drive the btf2go validation runner end-to-end on a
Proxmox VM cloned from the template built by `packer/`.

## Prerequisites

| | |
|---|---|
| Template | VMID 9100 (`btf2go-validation-tmpl`) — build with `packer/Makefile` |
| Repo `.env` | `PROXMOX_API_URL`, `PROXMOX_API_TOKEN` (`user@realm!tokenid=secret`) |
| Host tools | `curl`, `jq`, `python3`, `ssh`, `gh` (for the private repo token) |
| SSH key | `~/.ssh/id_ed25519` (override with `PX_SSH_KEY` / `PX_SSH_KEY_PUB`) |

Optional `.env` overrides:

```sh
PROXMOX_NODE=srv               # which Proxmox node hosts the template
PROXMOX_TEMPLATE_VMID=9100     # template to clone
PROXMOX_VMID_RANGE_LO=9110     # first VMID to use for clones
PROXMOX_VMID_RANGE_HI=9199     # last VMID to use for clones
PX_SSH_USER=dani               # cloud-init username on the clone
BTF2GO_GH_TOKEN=ghp_...        # explicit token; otherwise `gh auth token`
```

## One-shot: run the full validation suite

```sh
cd validation/proxmox
./validate.sh                  # clone -> run all tiers -> fetch report -> destroy
./validate.sh --tier 2         # T2 only
./validate.sh --keep           # leave the clone running so you can poke at it
./validate.sh --branch feature/foo --tier 2 --tier 4
```

Reports land in `validation/proxmox/reports/<vmid>-<timestamp>.md`.

## Step-by-step (when you want control)

```sh
# 1. Clone the template; prints "VMID IP" on stdout.
read VMID IP <<<"$(./clone.sh my-debug-vm)"

# 2. SSH in directly.
ssh dani@$IP

# 3. Or run the suite without destroying.
./run.sh $VMID --branch master --tier 2 --out /tmp/run.md

# 4. List active clones.
./list.sh

# 5. Tear it down.
./destroy.sh $VMID
./destroy.sh --all              # nuke every clone in the validation range
```

## Script overview

| Script | Job |
|---|---|
| `lib.sh` | shared helpers: env loading, API/curl wrapper, task polling, VMID allocation, IP discovery, SSH-key URL encoding (Proxmox quirk: double encode), `px_ssh` wrapper |
| `clone.sh [NAME]` | clone template -> attach cloud-init (user/SSH key/DHCP) -> boot -> wait for IP. Prints `VMID IP`. |
| `run.sh VMID\|--ip IP [--branch ...] [--tier ...] [--out ...]` | clone repo, build btf2go, run runner, scp the report back |
| `destroy.sh VMID...` or `destroy.sh --all` | stop + purge clones; refuses to touch the template |
| `list.sh` | tabular view of clones + the template, with status/uptime |
| `validate.sh [--branch ...] [--tier ...] [--keep]` | clone -> run -> destroy in one command. Default cleanup-on-exit; `--keep` leaves the VM up |

## How the SSH-key URL encoding works

Proxmox's `sshkeys` field is a known quirk: it expects the value to
arrive **already URL-encoded** (the API decodes it once internally
before validating). Sending raw "ssh-ed25519 AAAA…" gets rejected
with `invalid format - invalid urlencoded string`.

`lib.sh::px_sshkey_b64` double-URL-encodes the key so the post-decode
result is the once-encoded string Proxmox expects.

## How the cleanup contract is enforced

`validate.sh` registers a trap on EXIT that destroys the clone unless
`--keep` was passed. Crashes / Ctrl-C / non-zero exits all run the
trap. The template VMID is hard-protected in `destroy.sh`.

## Reporting bugs in the runner

The fetched `report.md` contains both PASS findings (sanity check)
and FAIL findings with per-struct mismatch details. File those as
issues against the runner; they're not infrastructure problems.
