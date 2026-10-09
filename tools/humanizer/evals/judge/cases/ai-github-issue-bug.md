---
id: ai-github-issue-bug
label: likely_ai
bucket: ai
split: test
source: generated
license: generated for this eval
generator: claude-sonnet-5-5 via claude -p, 2026-10-09
words: 172
notes: Bug-report template with bold field labels and code blocks; the two requested typos (regresion, Hapy) plant a fake human signal. Prompt was Write a GitHub issue reporting a bug in a Python CSV parsing library where quoted fields that contain newlines get split into two rows. Include a couple of typos. About 170 words. Output only the issue text, nothing else.
---
**Quoted fields containing newlines are split into two rows**

**Version:** 2.4.1
**Python:** 3.12
**OS:** Ubuntu 22.04

**Description**

When a quoted field contains a newline, the parser ends the row at the newline. It should keep reading until the closing quote. The result is two broken rows instead of one.

**Steps to reproduce**

```python
import csvkit_lite as csv

data = 'id,note\n1,"first line\nsecond line"\n2,ok\n'
rows = list(csv.reader(data.splitlines(keepends=True)))
print(rows)
```

**Expected**

```
[['id', 'note'], ['1', 'first line\nsecond line'], ['2', 'ok']]
```

**Actual**

```
[['id', 'note'], ['1', 'first line'], ['second line"'], ['2', 'ok']]
```

The second row is also left with a stray closing quote, so the data is silently corrupted and no error is raised.

**Notes**

This worked in 2.3.x, so it looks like a regresion in the new line-based tokenizer. Reading from a file opened with `newline=''` gives the same result. I think the quote state is being reset at the end of each line, but I haven't confirmed that. Hapy to send a PR if you can point me to the right module.
