# Corpus sources

Real flowcharts from the mermaid project (MIT licence, copyright the mermaid
contributors), fetched at commit b9b1759bbdcbf02bd082e78e9f39f5a26d16ffc1:

- docs-NN.mmd: every fenced flowchart in
  packages/mermaid/src/docs/syntax/flowchart.md
- demo-NN.mmd: every <pre class="mermaid"> flowchart in demos/flowchart.html

Refetch with the same regexes over those two files and keep only blocks
whose first statement is a graph or flowchart header.
