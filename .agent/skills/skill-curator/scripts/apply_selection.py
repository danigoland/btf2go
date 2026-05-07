#!/usr/bin/env python3
import argparse
import json
import os
import re
import shutil
from dataclasses import dataclass, asdict
from datetime import datetime, timezone
from pathlib import Path
from typing import Dict, List, Tuple

PROJECT_AGENT_PATHS: Dict[str, str] = {
    'antigravity': '.agent/skills',
    'claude-code': '.claude/skills',
    'codex': '.agents/skills',
    'gemini-cli': '.agents/skills',
    'windsurf': '.windsurf/skills',
    'droid': '.factory/skills',
}

DEFAULT_SYNC_AGENTS = ['antigravity', 'claude-code', 'codex', 'gemini-cli', 'windsurf', 'droid']


@dataclass
class Selection:
    name: str
    category: str
    reason: str


def load_skill_dir(catalog_root: Path, skill_name: str) -> Path:
    candidates = [
        catalog_root / 'skills' / skill_name,
        catalog_root / skill_name,
        catalog_root / '.agents' / 'skills' / skill_name,
    ]
    for candidate in candidates:
        if candidate.is_dir():
            return candidate
    raise FileNotFoundError(f"Skill '{skill_name}' not found under {catalog_root}")


def read_catalog_entries(catalog_root: Path) -> Tuple[Dict[str, str], Path | None]:
    candidates = [catalog_root / 'CATALOG.md', catalog_root / 'catalog.md']
    catalog_path = next((p for p in candidates if p.is_file()), None)
    if catalog_path is None:
        return {}, None
    text = catalog_path.read_text(encoding='utf-8', errors='replace')
    entries: Dict[str, str] = {}

    patterns = [
        re.compile(r'^[-*]\s+`([^`]+)`\s*[-:–—]?\s*(.*)$'),
        re.compile(r'^#+\s+`?([^`#]+?)`?\s*$'),
        re.compile(r'^\|\s*`?([^`|]+?)`?\s*\|\s*(.*?)\s*\|?$'),
        re.compile(r'^\d+\.\s+`([^`]+)`\s*[-:–—]?\s*(.*)$'),
    ]

    for raw_line in text.splitlines():
        line = raw_line.strip()
        if not line or line.startswith('<!--'):
            continue
        for pattern in patterns:
            match = pattern.match(line)
            if not match:
                continue
            name = match.group(1).strip().strip('/')
            desc = match.group(2).strip() if match.lastindex and match.lastindex >= 2 else ''
            if '/' in name or ' ' in name or name.lower() == 'name':
                # permit spaces in display names from tables, but normalize later
                pass
            if name and name not in entries:
                entries[name] = desc
            break
    return entries, catalog_path


def normalize_catalog_map(entries: Dict[str, str]) -> Dict[str, str]:
    normalized: Dict[str, str] = {}
    for name in entries:
        normalized[name.casefold()] = name
        normalized[name.replace(' ', '-').casefold()] = name
        normalized[name.replace('_', '-').casefold()] = name
    return normalized


def validate_against_catalog(skill_name: str, catalog_map: Dict[str, str]) -> str:
    if not catalog_map:
        return skill_name
    key_variants = {
        skill_name.casefold(),
        skill_name.replace(' ', '-').casefold(),
        skill_name.replace('_', '-').casefold(),
    }
    for key in key_variants:
        if key in catalog_map:
            return catalog_map[key]
    raise ValueError(f"Skill '{skill_name}' was not found in CATALOG.md")


def ensure_clean_target(target: Path) -> None:
    if target.is_symlink() or target.is_file():
        target.unlink()
    elif target.is_dir():
        shutil.rmtree(target)


def install_skill(source: Path, target: Path, copy_mode: str) -> None:
    ensure_clean_target(target)
    target.parent.mkdir(parents=True, exist_ok=True)
    if copy_mode == 'copy':
        shutil.copytree(source, target)
    else:
        os.symlink(source, target, target_is_directory=True)


def maybe_write_project_policy(project_root: Path, policy_source: Path) -> Path:
    target = project_root / '.agents' / 'curator-policy.yaml'
    if not target.exists():
        target.write_text(policy_source.read_text(encoding='utf-8'), encoding='utf-8')
    return target


def sync_agent_views(project_root: Path, selected_dir: Path, agents: List[str], copy_mode: str) -> List[dict]:
    results: List[dict] = []
    for agent in agents:
        rel_path = PROJECT_AGENT_PATHS.get(agent)
        if not rel_path:
            results.append({'agent': agent, 'status': 'skipped', 'reason': 'unknown project path'})
            continue
        target_root = project_root / rel_path
        target_root.mkdir(parents=True, exist_ok=True)
        synced = []
        for child in sorted(selected_dir.iterdir()):
            if child.name.startswith('.'):
                continue
            target = target_root / child.name
            install_skill(child, target, copy_mode)
            synced.append(child.name)
        results.append({'agent': agent, 'status': 'synced', 'path': str(target_root), 'skills': synced})
    return results


def write_lockfile(project_root: Path, catalog_root: Path, catalog_path: Path | None, copy_mode: str, selections: List[Selection], detected: dict, plan_files: List[str], sync_agents: List[str], sync_results: List[dict], policy_path: Path) -> Path:
    lock_path = project_root / '.agents' / 'skills.lock.json'
    payload = {
        'selection_policy_version': 5,
        'generated_at': datetime.now(timezone.utc).isoformat(),
        'catalog_root': str(catalog_root),
        'catalog_path': str(catalog_path) if catalog_path else None,
        'copy_mode': copy_mode,
        'detected': detected,
        'plan_files': plan_files,
        'policy_path': str(policy_path),
        'sync_agents': sync_agents,
        'sync_results': sync_results,
        'selected_skills': [asdict(s) for s in selections],
    }
    lock_path.write_text(json.dumps(payload, indent=2) + '\n', encoding='utf-8')
    return lock_path


def write_summary(project_root: Path, selections: List[Selection], detected: dict, sync_results: List[dict], policy_path: Path, catalog_path: Path | None) -> Path:
    summary_path = project_root / '.agents' / 'SKILLS.md'
    groups: Dict[str, List[Selection]] = {}
    for sel in selections:
        groups.setdefault(sel.category, []).append(sel)
    lines = ['# Selected skills', '', '## Detected project traits']
    for key, value in detected.items():
        lines.append(f'- **{key}**: {value}')
    lines.extend(['', '## Curation metadata'])
    lines.append(f'- **Policy**: `{policy_path}`')
    lines.append(f'- **Catalog**: `{catalog_path}`' if catalog_path else '- **Catalog**: not found')
    for category in sorted(groups):
        lines.extend(['', f'## {category.title()}'])
        for sel in groups[category]:
            lines.append(f'- **{sel.name}** — {sel.reason}')
    lines.extend(['', '## Agent sync'])
    for item in sync_results:
        if item['status'] == 'synced':
            lines.append(f"- **{item['agent']}** → `{item['path']}` ({len(item['skills'])} skills)")
        else:
            lines.append(f"- **{item['agent']}** — skipped: {item['reason']}")
    summary_path.write_text('\n'.join(lines) + '\n', encoding='utf-8')
    return summary_path


def write_rerun(project_root: Path, catalog_root: Path, copy_mode: str, plan_files: List[str], sync_agents: List[str]) -> Path:
    rerun_path = project_root / '.agents' / 'rerun-curator.sh'
    lines = [
        '#!/usr/bin/env bash',
        'set -euo pipefail',
        'PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"',
        f'CATALOG_ROOT="{catalog_root}"',
        f'COPY_MODE="{copy_mode}"',
        'SCRIPT_DIR="$PROJECT_ROOT/.agents/selected-skills/skill-curator/scripts"',
        'if [ ! -d "$SCRIPT_DIR" ]; then',
        '  echo "Could not locate skill-curator scripts under .agents/selected-skills" >&2',
        '  exit 1',
        'fi',
        'CMD=("$SCRIPT_DIR/run_curator.sh" "$PROJECT_ROOT" "--catalog-root" "$CATALOG_ROOT" "--copy-mode" "$COPY_MODE")',
    ]
    for agent in sync_agents:
        lines.append(f'CMD+=("--sync-agent" "{agent}")')
    for pf in plan_files:
        lines.append(f'CMD+=("--plan-file" "{pf}")')
    lines.append('"${CMD[@]}"')
    rerun_path.write_text('\n'.join(lines) + '\n', encoding='utf-8')
    rerun_path.chmod(0o755)
    return rerun_path


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument('--project-root', required=True)
    parser.add_argument('--catalog-root', required=True)
    parser.add_argument('--copy-mode', choices=['copy', 'symlink'], default='copy')
    parser.add_argument('--detected-json', default='{}')
    parser.add_argument('--selection-json', default='[]')
    parser.add_argument('--plan-file', action='append', default=[])
    parser.add_argument('--skill', action='append', default=[])
    parser.add_argument('--sync-agent', action='append', default=[])
    args = parser.parse_args()

    project_root = Path(args.project_root).resolve()
    catalog_root = Path(args.catalog_root).expanduser().resolve()
    agents_dir = project_root / '.agents'
    selected_dir = agents_dir / 'selected-skills'
    agents_dir.mkdir(parents=True, exist_ok=True)
    selected_dir.mkdir(parents=True, exist_ok=True)

    detected = json.loads(args.detected_json)
    selections_payload = json.loads(args.selection_json)
    if selections_payload:
        selections = [Selection(**item) for item in selections_payload]
    else:
        selections = [Selection(name=s, category='selected', reason='selected by curator') for s in args.skill]
    if not selections:
        raise SystemExit('No skills provided. Pass --skill or --selection-json.')

    catalog_entries, catalog_path = read_catalog_entries(catalog_root)
    catalog_map = normalize_catalog_map(catalog_entries)
    canonical_selections: List[Selection] = []
    seen = set()
    for selection in selections:
        canonical_name = validate_against_catalog(selection.name, catalog_map)
        if canonical_name.casefold() in seen:
            continue
        seen.add(canonical_name.casefold())
        canonical_selections.append(Selection(name=canonical_name, category=selection.category, reason=selection.reason))

    for selection in canonical_selections:
        source = load_skill_dir(catalog_root, selection.name)
        target = selected_dir / selection.name
        install_skill(source, target, args.copy_mode)

    curator_source = Path(__file__).resolve().parents[1]
    curator_target = selected_dir / 'skill-curator'
    if not (curator_target.exists() and curator_target.resolve() == curator_source.resolve()):
        install_skill(curator_source, curator_target, 'copy')

    policy_source = curator_source / 'references' / 'selection-policy.yaml'
    project_policy = maybe_write_project_policy(project_root, policy_source)

    sync_agents = args.sync_agent or DEFAULT_SYNC_AGENTS
    sync_results = sync_agent_views(project_root, selected_dir, sync_agents, args.copy_mode)

    write_lockfile(project_root, catalog_root, catalog_path, args.copy_mode, canonical_selections, detected, args.plan_file, sync_agents, sync_results, project_policy)
    write_summary(project_root, canonical_selections, detected, sync_results, project_policy, catalog_path)
    write_rerun(project_root, catalog_root, args.copy_mode, args.plan_file, sync_agents)


if __name__ == '__main__':
    main()
