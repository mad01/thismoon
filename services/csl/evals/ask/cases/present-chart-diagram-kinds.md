---
id: present-chart-diagram-kinds
title: Chart and diagram vocabularies in the renderer
split: train
query: 'chartKinds|diagramKinds'
file: 'render/doc\.go$'
anchors: [services/present/internal/render/doc.go]
max_files: 1
max_answer_chars: 900
expected:
  - '\bbar\b'
  - '\bline\b'
  - '\barea\b'
  - 'sparkline'
  - 'stacked-bar'
  - 'horizontal-bar'
  - 'doughnut'
  - 'scatter'
  - 'sankey'
  - 'ribbon'
  - '\bservice\b'
  - '\bstore\b'
  - '\bqueue\b'
  - '\bperson\b'
  - '\bexternal\b'
  - '\bLR\b'
  - '\bTB\b'
forbidden:
  - '\bpie\b'
  - '\bradar\b'
  - '\bRL\b'
  - '\bBT\b'
  - '\bpolar\b'
oracle: |
  - chartKinds (doc.go:66): bar, line, area, sparkline, stacked-bar, horizontal-bar, doughnut, scatter, sankey, ribbon. An empty kind draws as bar.
  - diagramKinds (doc.go:78): service, store, queue, person, external. An empty kind draws as service.
  - diagramDirections (doc.go:81): LR (the default) and TB.
notes: >-
  The largest file in the repo (1998 lines). The answer is three literal
  lists near the top; the test is whether the worker copies them exactly
  and resists the chart kinds every charting library has but this one
  refuses.
---
Which chart kinds, diagram node kinds, and diagram directions does this renderer accept? Give the exact lists and the default for each.
