// The graph bundle the brain.this UI loads before graph.js and app.js
// (docs/serve.md, Browser globals). build.mjs bundles this file into one
// IIFE, so the page needs no module loader and no CDN.
import Graph from 'graphology';
import Sigma from 'sigma';
import forceAtlas2 from 'graphology-layout-forceatlas2';
import FA2Layout from 'graphology-layout-forceatlas2/worker';
import circular from 'graphology-layout/circular';
import MarkdownIt from 'markdown-it';

window.BrainSigma = { Graph, Sigma, forceAtlas2, FA2Layout, circular, MarkdownIt };
