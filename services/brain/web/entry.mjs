// The graph bundle the brain.this UI loads before graph.js and app.js
// (docs/serve.md, Browser globals). build.mjs bundles this file into one
// IIFE, so the page needs no module loader and no CDN.
//
// sigma 4 draws with the primitives a page declares, so the node and edge
// pieces app.js composes (circle, fill, border, line, curve, loop, arrow)
// ship next to sigma itself. d3-force runs the live layout, and Louvain
// finds the communities the global layout groups the notes by.
import Graph from 'graphology';
import Sigma from 'sigma';
import {
  extremityArrow,
  layerFill,
  layerPlain,
  pathCurved,
  pathLine,
  pathLoop,
  sdfCircle,
} from 'sigma/rendering';
import { layerBorder } from '@sigma/node-border';
import louvain from 'graphology-communities-louvain';
import { forceCollide, forceLink, forceManyBody, forceSimulation, forceX, forceY } from 'd3-force';
import MarkdownIt from 'markdown-it';

window.BrainSigma = {
  Graph,
  Sigma,
  MarkdownIt,
  sdfCircle,
  layerFill,
  layerBorder,
  pathLine,
  pathCurved,
  pathLoop,
  extremityArrow,
  layerPlain,
  forceSimulation,
  forceLink,
  forceManyBody,
  forceCollide,
  forceX,
  forceY,
  louvain,
};
