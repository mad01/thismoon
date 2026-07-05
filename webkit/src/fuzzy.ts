// fuzzy.ts — pure subsequence matcher for the ⌘K site picker.
// fuzzyMatch() scores a target against a query; rankSites() orders a site list
// best-first and returns the matched character indices so the UI can highlight
// them. No DOM here — kept pure so it's unit-testable (cf. bionic.ts, size.ts).

export interface Site {
  name: string;
  host?: string;
  url: string;
}

export interface Match {
  score: number;
  indices: number[];
}

export interface SiteMatch {
  site: Site;
  indices: number[]; // matched positions in site.name (for <mark> highlighting)
}

// fuzzyMatch returns a score + matched indices when every char of query appears
// in target in order (case-insensitive subsequence), else null. Higher score is
// a better match: consecutive runs, start-of-string, and word-boundary hits all
// score higher; longer targets and a later first hit are penalised slightly.
export function fuzzyMatch(query: string, target: string): Match | null {
  const q = query.toLowerCase();
  const t = target.toLowerCase();
  if (q === '') return { score: 0, indices: [] };

  const indices: number[] = [];
  let score = 0;
  let ti = 0;
  let prev = -1;
  for (const ch of q) {
    let found = -1;
    for (; ti < t.length; ti++) {
      if (t[ti] === ch) { found = ti; ti++; break; }
    }
    if (found === -1) return null; // a query char is missing (or out of order)
    indices.push(found);
    score += 1;
    if (found === prev + 1) score += 3;                   // consecutive run
    if (found === 0) score += 5;                          // start of string
    else if (!/[a-z0-9]/.test(t[found - 1])) score += 2;  // word boundary
    prev = found;
  }
  score -= t.length * 0.05;   // prefer shorter targets, all else equal
  score -= indices[0] * 0.1;  // prefer an earlier first hit
  return { score, indices };
}

// rankSites filters sites to those matching query and orders them best-first.
// It matches on name first, then falls back to host/url so "this" or a port
// still finds a site. An empty query returns every site in its original order.
export function rankSites(query: string, sites: Site[]): SiteMatch[] {
  if (query.trim() === '') return sites.map(site => ({ site, indices: [] }));

  const scored: { match: SiteMatch; score: number; order: number }[] = [];
  sites.forEach((site, order) => {
    const byName = fuzzyMatch(query, site.name);
    if (byName) {
      // Name hits rank above host-only hits.
      scored.push({ match: { site, indices: byName.indices }, score: byName.score + 1, order });
      return;
    }
    const byHost = fuzzyMatch(query, site.host ?? site.url);
    if (byHost) {
      scored.push({ match: { site, indices: [] }, score: byHost.score, order });
    }
  });

  scored.sort((a, b) => b.score - a.score || a.order - b.order);
  return scored.map(s => s.match);
}
