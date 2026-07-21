#!/usr/bin/env python3
# i18n-validate: Verify all locale files have the same keys as en.json.
# AI.md PART 31 - Build-time validation requirement.
# Usage: python3 scripts/i18n-validate.sh [locales_dir]
# Exit code: 0 = pass, 1 = fail

import json
import os
import sys

locale_dir = sys.argv[1] if len(sys.argv) > 1 else "src/common/i18n/locales"

if not os.path.isdir(locale_dir):
    print(f"ERROR: locale directory not found: {locale_dir}")
    sys.exit(1)

en_path = os.path.join(locale_dir, "en.json")
if not os.path.isfile(en_path):
    print(f"ERROR: en.json not found in {locale_dir}")
    sys.exit(1)

with open(en_path, encoding="utf-8") as f:
    en_keys = set(json.load(f).keys())

print(f"en.json: {len(en_keys)} keys")

failed = False
for fname in sorted(os.listdir(locale_dir)):
    if not fname.endswith(".json") or fname == "en.json":
        continue
    lang = fname[:-5]
    path = os.path.join(locale_dir, fname)
    with open(path, encoding="utf-8") as f:
        keys = set(json.load(f).keys())
    missing = en_keys - keys
    extra = keys - en_keys
    status = "✅" if not missing and not extra else "❌"
    print(f"{status} {lang}.json: {len(keys)} keys", end="")
    if missing:
        print(f" | missing {len(missing)}: {sorted(missing)}", end="")
        failed = True
    if extra:
        print(f" | extra {len(extra)}: {sorted(extra)}", end="")
        failed = True
    print()

if failed:
    print("\nFAIL: locale key mismatch - all languages must have identical keys to en.json")
    sys.exit(1)

print("\nPASS: all locale files have matching keys")
sys.exit(0)
