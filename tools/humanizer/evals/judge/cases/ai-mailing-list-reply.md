---
id: ai-mailing-list-reply
label: likely_ai
bucket: ai
split: test
source: generated
license: generated for this eval
generator: google/gemini-3.8-flash via OpenRouter chat completions, 2026-10-09
words: 209
notes: Mailing-list reply with a [Name] placeholder, bold-lead numbered causes, LaTeX big-O and a sign-off. Prompt was Write a reply on the users mailing list of a small open-source static site generator. The question was why a full rebuild of a site with 8000 pages takes four minutes and whether there is a faster way. About 200 words. Output only the reply, nothing else.
---
Hi [Name],

Four minutes for 8,000 pages works out to about 30ms per page. While not completely terrible for a single-threaded cold build, there is definitely room to speed that up.

A few common culprits usually cause slowdowns at this scale:

1. **Expensive template logic:** If your templates loop over site-wide collections (e.g., calculating "related posts" or rendering tag clouds) inside every single page, rendering complexity becomes quadratic ($O(n^2)$). Try moving that logic to site-level data files rather than computing it per page.
2. **Asset copying / processing:** Check if the builder is reprocessing or copying static assets (like images) on every run.
3. **Disk I/O:** Writing 8,000 individual HTML files sequentially hits the filesystem hard, especially on spinning disks or within certain container environments.

Ways to make it faster:

- **Incremental builds:** For local editing, use the `--incremental` (or `--dirty`) flag so only modified Markdown files are re-rendered.
- **Parallel rendering:** In the latest release, you can use `--workers auto` to split the page rendering across available CPU cores.
- **Build to memory:** In CI, writing to a RAM disk (`tmpfs`) can easily cut your build time in half.

Could you share your config file or run with `--profile`? That would help pinpoint the exact bottleneck.

Best,
Alex
