// Deterministic, dependency-free force-directed layout (Fruchterman–Reingold).
// The model is a graph (ops→faults, truths→invariants, invariants→specs, …) but
// the column view only implies those links via dimming; GraphView draws them.
//
// The layout is a pure function of (nodeIds, edges): no Math.random, fixed
// iteration count and cooling schedule, circle-seeded initial positions. Same
// model in => identical coordinates out, so it matches the project's determinism
// ethos and is unit-testable.

export interface XY {
  x: number;
  y: number;
}

export interface LayoutOptions {
  width?: number;
  height?: number;
  iterations?: number;
  margin?: number;
}

export interface LayoutResult {
  /** Node id -> position, normalised to fit [margin, size-margin]. */
  pos: Map<string, XY>;
  width: number;
  height: number;
}

/**
 * computeLayout places nodeIds in a width×height canvas so connected nodes sit
 * near each other and unconnected ones repel. Edges to ids not in nodeIds (and
 * self-edges) are ignored, so the caller can pass the raw edge list.
 */
export function computeLayout(
  nodeIds: readonly string[],
  edges: readonly { from: string; to: string }[],
  opts: LayoutOptions = {},
): LayoutResult {
  const width = opts.width ?? 1000;
  const height = opts.height ?? 700;
  const iterations = opts.iterations ?? 500;
  const margin = opts.margin ?? 40;
  const n = nodeIds.length;
  if (n === 0) return { pos: new Map(), width, height };
  if (n === 1) return { pos: new Map([[nodeIds[0], { x: width / 2, y: height / 2 }]]), width, height };

  const idx = new Map<string, number>();
  nodeIds.forEach((id, i) => idx.set(id, i));

  // Deterministic scatter seed (seeded PRNG — no Math.random). A pseudo-random
  // start breaks the symmetry a circle seed leaves behind (which collapses into
  // an arc), letting the simulation find a balanced layout.
  const rand = mulberry32(0x9e3779b9);
  const xs = new Float64Array(n);
  const ys = new Float64Array(n);
  for (let i = 0; i < n; i++) {
    xs[i] = rand() * width;
    ys[i] = rand() * height;
  }
  const cx = width / 2;
  const cy = height / 2;
  const gravity = 0.06; // gentle centering pull so leaves don't fly to the rim

  // Resolve edges to index pairs once.
  const pairs: Array<[number, number]> = [];
  for (const e of edges) {
    const a = idx.get(e.from);
    const b = idx.get(e.to);
    if (a === undefined || b === undefined || a === b) continue;
    pairs.push([a, b]);
  }

  const area = width * height;
  const k = Math.sqrt(area / n); // ideal node separation
  const dx = new Float64Array(n);
  const dy = new Float64Array(n);
  let temp = width / 10;
  const cool = temp / (iterations + 1);

  for (let it = 0; it < iterations; it++) {
    dx.fill(0);
    dy.fill(0);

    // Repulsion between every pair: fr = k^2 / d.
    for (let i = 0; i < n; i++) {
      for (let j = i + 1; j < n; j++) {
        let ddx = xs[i] - xs[j];
        let ddy = ys[i] - ys[j];
        let dist = Math.hypot(ddx, ddy);
        if (dist < 0.01) {
          // Coincident: nudge deterministically by index so they separate.
          ddx = (i - j) * 0.01 + 0.01;
          ddy = (i + j) * 0.01 + 0.01;
          dist = Math.hypot(ddx, ddy);
        }
        const rep = (k * k) / dist;
        const ux = ddx / dist;
        const uy = ddy / dist;
        dx[i] += ux * rep;
        dy[i] += uy * rep;
        dx[j] -= ux * rep;
        dy[j] -= uy * rep;
      }
    }

    // Attraction along edges: fa = d^2 / k.
    for (const [a, b] of pairs) {
      let ddx = xs[a] - xs[b];
      let ddy = ys[a] - ys[b];
      const dist = Math.hypot(ddx, ddy) || 0.01;
      const att = (dist * dist) / k;
      const ux = ddx / dist;
      const uy = ddy / dist;
      dx[a] -= ux * att;
      dy[a] -= uy * att;
      dx[b] += ux * att;
      dy[b] += uy * att;
    }

    // Centering gravity: pull every node toward the middle so disconnected and
    // leaf nodes settle inward instead of drifting to the edges.
    for (let i = 0; i < n; i++) {
      dx[i] += (cx - xs[i]) * gravity;
      dy[i] += (cy - ys[i]) * gravity;
    }

    // Move, capped by the cooling temperature.
    for (let i = 0; i < n; i++) {
      const disp = Math.hypot(dx[i], dy[i]) || 0.01;
      const lim = Math.min(disp, temp);
      xs[i] += (dx[i] / disp) * lim;
      ys[i] += (dy[i] / disp) * lim;
    }
    temp -= cool;
  }

  // Normalise the final cloud into [margin, size - margin], preserving aspect.
  let minX = Infinity;
  let minY = Infinity;
  let maxX = -Infinity;
  let maxY = -Infinity;
  for (let i = 0; i < n; i++) {
    if (xs[i] < minX) minX = xs[i];
    if (xs[i] > maxX) maxX = xs[i];
    if (ys[i] < minY) minY = ys[i];
    if (ys[i] > maxY) maxY = ys[i];
  }
  const spanX = maxX - minX || 1;
  const spanY = maxY - minY || 1;
  const scale = Math.min((width - 2 * margin) / spanX, (height - 2 * margin) / spanY);

  const pos = new Map<string, XY>();
  for (let i = 0; i < n; i++) {
    pos.set(nodeIds[i], {
      x: margin + (xs[i] - minX) * scale,
      y: margin + (ys[i] - minY) * scale,
    });
  }
  return { pos, width, height };
}

// mulberry32: a tiny deterministic PRNG for the initial scatter. Seeded with a
// constant so computeLayout stays a pure function of its inputs.
function mulberry32(seed: number): () => number {
  let s = seed >>> 0;
  return () => {
    s = (s + 0x6d2b79f5) | 0;
    let t = Math.imul(s ^ (s >>> 15), 1 | s);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}
