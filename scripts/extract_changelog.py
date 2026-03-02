#!/usr/bin/env python3
import os
import re
import sys

body = os.environ.get("PR_BODY") or ""
lines = body.splitlines()

wanted = ["added", "changed", "removed"]
sections = {k: [] for k in wanted}
current = None

hdr = re.compile(r'^###\s*(Added|Changed|Removed)\s*$', re.I)

for ln in lines:
    m = hdr.match(ln)
    if m:
        current = m.group(1).lower()
        continue
    if current:
        sections[current].append(ln)

out = []
for key in wanted:
    content = [l for l in sections[key] if l.strip()]
    if not content:
        continue
    out.append(f"### {key.capitalize()}")
    out.extend(content)

result = "\n".join(out).rstrip("\n")
if not result.strip():
    print("", end="")
    sys.exit(0)

print(result)