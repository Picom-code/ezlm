# T13 a15: assembly script

## Summary

`agents/T13/assemble.py` merges `agents/T13/sections/aNN.md` into the three
beta documents in one deterministic pass:

| Output | Sections, in order |
| --- | --- |
| `agents/T13/BETA.md` | a01, a02, a03, a04, a05, a06, a07, a08 |
| `agents/T13/TROUBLESHOOTING.md` | a09, a10, a11, a12 |
| `agents/T13/FAQ.md` | a13 |

It also checks every troubleshooting entry for the three required parts (the
message, what it means, what to do) and reports the ones that are missing.

`agents/T13/a15/self-test.sh` builds a fixture tree in a temp directory and
checks the merge, the gap report and idempotency. Nothing outside the temp
directory is touched.

## How the Lead runs it

From anywhere, after the sections are in `agents/T13/sections/`:

```sh
# 1. See what is missing without writing anything. Exit 1 means gaps.
python3 agents/T13/assemble.py --check

# 2. Write the three documents and a gap report for the record.
python3 agents/T13/assemble.py --report agents/T13/a15/assembly-report.md
```

Exit codes: `0` wrote the documents (gaps are printed but not fatal), `1` gaps
under `--check` or `--strict`, `2` bad invocation (no section directory,
duplicate `aNN` file). `--list` prints the sections that were found, which is
the quickest way to see which agents have delivered. `--quiet` prints only
problems. `--no-demote` keeps section heading levels exactly as written.

Missing sections are warnings, not errors, so the Lead can assemble partial
work at any point. A rerun writes the same bytes (`assemble.py:write_if_changed`),
so assembling twice does not create an empty commit.

## Entry format for a09 to a12

The script documents this in its docstring and matches it loosely:

```markdown
## "Windows protected your PC"

**Message:** Windows protected your PC
**What it means:** This beta is not code-signed yet, so SmartScreen warns.
**What to do:** Click More info, then Run anyway.
```

- The heading counts as the message when it quotes the text, so the
  `**Message:**` line is optional.
- Labels are normalised and matched against aliases (`assemble.py:ALIASES`):
  "What it means" / "Why it happens" / "Cause" all count as the meaning;
  "What to do" / "Fix" / "Next step" all count as the action.
- A label can be bold, italic, a heading, or a bullet, with the text on the
  same line or the lines below it.
- A three-column table with `Message`, `What it means` and `What to do`
  headers is accepted when the section has no per-entry headings
  (`assemble.py:table_entries`). It is skipped when headings already produced
  entries, so an entry is not counted twice.

## What was tested

`sh agents/T13/a15/self-test.sh` passes. It covers:

- a01 to a08 merged into `BETA.md` in numeric order, with `# Section` demoted
  to `## Section` so the document has one h1 (`assemble.py:demote_headings`);
- a09 to a12 merged into `TROUBLESHOOTING.md` in numeric order;
- the quoted-heading form, the labelled form with the text on following
  lines, the "Why it happens" / "Next step" aliases, and the table form;
- a missing action reported as exactly one gap, at `a09.md:9`;
- missing sections reported as warnings and not written as empty files;
- duplicate messages across sections reported as a warning;
- a rerun producing identical bytes.

Also checked by hand: `python3 -m py_compile`, `--help`, `--list`, an empty
section directory (exit 2), and a partially delivered set (only a01, a09, a10:
`BETA.md` and `TROUBLESHOOTING.md` written, `FAQ.md` skipped with a warning).

## Findings and limitations

- **Minor, by design:** the message part is inferred from the entry heading
  when the heading is not generic (`assemble.py:GENERIC_HEADINGS`). If an agent
  groups entries under a heading like "Engine" and puts the exact text only in
  prose without a label, the entry is reported as missing the message. That is
  the check doing its job: the exact text should be quoted or labelled.
- **Minor:** duplicate detection normalises case and surrounding punctuation
  only, so "Cannot reach the engine" and "Cannot reach the engine." count as
  different messages.
- **Cosmetic:** heading demotion is per section. A section that starts with an
  h1 gets all its headings moved down one level; a section with no h1 keeps its
  levels, so a document can mix `##` and `###` entry depths. This does not
  affect rendering or anchors.
- **Cosmetic:** if a section's own h1 repeats the document title (a13 writing
  `# FAQ`), the assembled file shows `# Frequently asked questions` followed by
  `## FAQ`. Harmless, but the Lead may want a13 to drop its h1.
- Not done: a table of contents for `BETA.md`, and any check of the guide or
  FAQ sections beyond "present and non-empty". The a14 unclear-message list and
  the a16 review are not part of the three documents and are not assembled.
- The generated files carry an HTML comment saying they are generated. If the
  Lead hand-edits them, the next run overwrites the edit.

## Open questions

- Should `BETA.md` replace the repository's root `BETA.md`, or live beside it
  as the fuller guide? The brief says "you may propose a new `agents/T13/BETA.md`",
  so this script writes only under `agents/T13/` and leaves the root file alone.
- Should a gap fail the Lead's run (`--strict`)? The default is to report and
  keep going, because sections arrive over the night.
