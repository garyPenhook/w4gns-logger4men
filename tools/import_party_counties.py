#!/usr/bin/env python3
"""Extract a checked sponsor county table; emit JSON without changing the catalog.

Usage: python3 tools/import_party_counties.py --format cells --count 83 page.html
Only table facts are retained. Download the sponsor page separately and verify
its rules, abbreviation column and expected county count before importing.
"""

import argparse
import json
import re
from html.parser import HTMLParser
from pathlib import Path


class TableText(HTMLParser):
    def __init__(self):
        super().__init__(convert_charrefs=True)
        self.rows = []
        self.text = []
        self.row = []
        self.cell = None
        self.skip = 0

    def handle_starttag(self, tag, attrs):
        if tag in ("script", "style"):
            self.skip += 1
        if self.skip:
            return
        if tag == "tr":
            self.row = []
        elif tag == "td":
            self.cell = []
        elif tag in ("br", "p", "div"):
            self.text.append("\n")

    def handle_endtag(self, tag):
        if tag in ("script", "style"):
            self.skip = max(0, self.skip - 1)
            return
        if self.skip:
            return
        if tag == "td" and self.cell is not None:
            self.row.append(" ".join("".join(self.cell).split()))
            self.cell = None
        elif tag == "tr":
            self.rows.append(self.row)
        elif tag in ("p", "div"):
            self.text.append("\n")

    def handle_data(self, data):
        if not self.skip:
            self.text.append(data)
            if self.cell is not None:
                self.cell.append(data)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--format", choices=("cells", "name_cells", "equals", "numbered"), required=True)
    parser.add_argument("--count", type=int, required=True)
    parser.add_argument("page", type=Path)
    args = parser.parse_args()
    page = TableText()
    page.feed(args.page.read_text(encoding="utf-8"))
    if args.format == "cells":
        pairs = [(r[0], r[1]) for r in page.rows if len(r) == 2]
    elif args.format == "name_cells":
        pairs = []
        for row in page.rows:
            cells = [c for c in row if c]
            if len(cells) == 3 and cells[0] == cells[2]:
                cells.pop()
            if len(cells) % 2 == 0:
                pairs.extend((cells[i + 1], cells[i]) for i in range(0, len(cells), 2))
    elif args.format == "numbered":
        pairs = re.findall(r"(?:^|\s{2,})\d+\s+([A-Z]{3})\s+(.+?)(?=\s{2,}\d+\s|$)", "".join(page.text), re.M)
    else:
        pairs = re.findall(r"^\s*([A-Z]{3,5})\s*=\s*([^,\n]+)", "".join(page.text), re.M)
    counties = {}
    for code, name in pairs:
        if not re.fullmatch(r"[A-Z]{3,5}", code):
            continue
        name = name.strip().title()
        if code in counties and counties[code] != name:
            raise ValueError(f"Conflicting county names for {code}")
        counties[code] = name
    if len(counties) != args.count:
        raise ValueError(f"Expected {args.count} counties; extracted {len(counties)}")
    print(json.dumps([{"code": k, "name": v} for k, v in sorted(counties.items())], indent=2))


if __name__ == "__main__":
    main()
