#!/usr/bin/env python3
import argparse
import json
import re
from pathlib import Path


def parse_catalog(catalog_path: Path) -> list[dict]:
    patterns = [
        re.compile(r'^[-*]\s+`([^`]+)`\s*[-:–—]?\s*(.*)$'),
        re.compile(r'^\d+\.\s+`([^`]+)`\s*[-:–—]?\s*(.*)$'),
        re.compile(r'^\|\s*`?([^`|]+?)`?\s*\|\s*(.*?)\s*\|?$'),
    ]
    items = []
    seen = set()
    for raw_line in catalog_path.read_text(encoding='utf-8', errors='replace').splitlines():
        line = raw_line.strip()
        for pattern in patterns:
            match = pattern.match(line)
            if match:
                name = match.group(1).strip()
                desc = match.group(2).strip() if match.lastindex and match.lastindex >= 2 else ''
                if name.casefold() not in seen:
                    seen.add(name.casefold())
                    items.append({'name': name, 'description': desc})
                break
    return items


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument('--catalog-path', required=True)
    args = parser.parse_args()
    path = Path(args.catalog_path).expanduser().resolve()
    if not path.is_file():
        raise SystemExit(f'Catalog not found: {path}')
    print(json.dumps(parse_catalog(path), indent=2))


if __name__ == '__main__':
    main()
