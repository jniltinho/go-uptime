#!/usr/bin/env python3
# Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.
"""Puts the one-line notice of the Apache-2.0 section 4(b) in every first-party file that can carry a comment.

Usage: license-notices.py <repo root> [--check]

The rule does not depend on which files came from Gatus: git cannot tell that reliably for files that were moved and
heavily rewritten, so every first-party file carries the notice. The files that cannot carry a comment are printed, to
be listed by path in NOTICE. The same rules are in internal/test/license_notice_test.go, which enforces them.
"""
import re
import subprocess
import sys

root = sys.argv[1]
check = "--check" in sys.argv
MARKER = "derived from Gatus by TwiN"
TEXT = "Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE."

EXCLUDED_PREFIXES = ("third_party/", "openspec/", ".claude/commands/", "web/static/", "web/app/src/assets/fonts/", "web/app/public/fonts/")
EXCLUDED_FILES = {"LICENSE", "NOTICE", ".claude/skills/LICENSE.cc-skills-golang"}
OWN_SKILLS = (".claude/skills/create-release/",)

LINE = {".go": "//", ".mod": "//", ".js": "//", ".mjs": "//", ".cjs": "//", ".yaml": "#", ".yml": "#", ".sh": "#", ".py": "#",
        ".service": "#", ".conf": "#", ".toml": "#", ".ini": ";"}
BLOCK = {".css": ("/* ", " */"), ".vue": ("<!-- ", " -->"), ".html": ("<!-- ", " -->"), ".md": ("<!-- ", " -->"),
         ".svg": ("<!-- ", " -->"), ".xml": ("<!-- ", " -->"), ".drawio": ("<!-- ", " -->")}
HASH_NAMES = {"Makefile", "Dockerfile", "Dockerfile.release", ".gitignore", ".dockerignore", ".gitattributes", ".editorconfig",
              "Dockerfile.release.dockerignore", ".browserslistrc", ".npmrc"}
NO_COMMENT = (".json", ".sum", ".nvmrc", ".lock")


def excluded(name):
    if name in EXCLUDED_FILES or name.startswith(EXCLUDED_PREFIXES) or "/testdata/" in name or name.endswith((".gitkeep", ".crt", ".key", ".pem")):
        return True
    if name.startswith(".claude/skills/") and not name.startswith(OWN_SKILLS):
        return True  # imported skills must stay identical to their origin
    return False


def comment(name):
    base = name.rsplit("/", 1)[-1]
    extension = "." + base.rsplit(".", 1)[-1] if "." in base else ""
    if base in HASH_NAMES or base.startswith("Dockerfile"):
        return "# " + TEXT
    if extension in LINE:
        return f"{LINE[extension]} {TEXT}"
    if extension in BLOCK:
        return BLOCK[extension][0] + TEXT + BLOCK[extension][1]
    return None


def insert(name, text, notice):
    lines = text.split("\n")
    position = 0
    if lines and lines[0].startswith("#!"):
        position = 1
    elif name.endswith(".md") and lines and lines[0].strip() == "---":
        position = lines.index("---", 1) + 1 if "---" in lines[1:] else 0
    elif lines and re.match(r"(?i)\s*(<!doctype|<\?xml)", lines[0]):
        position = 1
    new = lines[:position] + [notice] + lines[position:]
    # Go: a blank line keeps the notice from becoming the package comment, and from touching a build constraint
    if name.endswith(".go") and new[position + 1].strip() != "":
        new.insert(position + 1, "")
    return "\n".join(new)


files = subprocess.check_output(["git", "-C", root, "ls-files", "-z"]).decode().rstrip("\0").split("\0")
missing, without_comment, changed = [], [], 0
for name in files:
    if excluded(name):
        continue
    path = f"{root}/{name}"
    try:
        text = open(path, encoding="utf-8").read()
    except (UnicodeDecodeError, FileNotFoundError, IsADirectoryError):
        continue  # binary, or removed in the working tree
    if name.endswith(NO_COMMENT):
        without_comment.append(name)
        continue
    notice = comment(name)
    if notice is None:
        missing.append(name + "  (format without a rule)")
        continue
    if MARKER in "\n".join(text.split("\n")[:12]):
        continue
    if check:
        missing.append(name)
        continue
    open(path, "w", encoding="utf-8").write(insert(name, text, notice))
    changed += 1

print(f"{changed} files changed; {len(missing)} without the notice; {len(without_comment)} that cannot carry a comment")
for name in missing:
    print("  missing:", name)
print("FILES WITHOUT COMMENT:")
for name in without_comment:
    print("  " + name)
sys.exit(1 if check and missing else 0)
