package api

import "net/http"

func serveUI(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(uiHTML))
}

const uiHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Mimir — Code Graph Explorer</title>
<style>
* { margin: 0; padding: 0; box-sizing: border-box; }
body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; background: #0d1117; color: #c9d1d9; overflow: hidden; height: 100vh; }

/* Top bar */
#topbar {
  display: flex; align-items: center; gap: 12px;
  padding: 8px 16px; background: #161b22; border-bottom: 1px solid #30363d; height: 48px; z-index: 10;
}
#topbar .logo { font-weight: 700; font-size: 18px; color: #58a6ff; letter-spacing: 1px; }
#topbar select, #topbar input {
  background: #0d1117; border: 1px solid #30363d; color: #c9d1d9; border-radius: 6px;
  padding: 6px 10px; font-size: 13px; outline: none;
}
#topbar select:focus, #topbar input:focus { border-color: #58a6ff; }
#topbar input { width: 220px; }
#stats { margin-left: auto; font-size: 12px; color: #8b949e; display: flex; gap: 12px; }
#stats span { display: inline-flex; align-items: center; gap: 4px; }

/* Main layout */
#main { display: flex; height: calc(100vh - 48px - 36px); }
#graph-container { flex: 1; position: relative; background: #0d1117; }
#sigma-container { width: 100%; height: 100%; }

/* Detail panel */
#detail {
  width: 320px; background: #161b22; border-left: 1px solid #30363d;
  overflow-y: auto; padding: 16px; display: none; flex-shrink: 0;
}
#detail.open { display: block; }
#detail h3 { color: #58a6ff; font-size: 15px; margin-bottom: 12px; display: flex; justify-content: space-between; align-items: center; }
#detail .close-btn { cursor: pointer; color: #8b949e; font-size: 18px; background: none; border: none; }
#detail .close-btn:hover { color: #c9d1d9; }
#detail .field { margin-bottom: 10px; }
#detail .field label { font-size: 11px; color: #8b949e; text-transform: uppercase; letter-spacing: 0.5px; display: block; margin-bottom: 2px; }
#detail .field .val { font-size: 13px; word-break: break-all; }
#detail .field .kind-badge {
  display: inline-block; padding: 2px 8px; border-radius: 10px; font-size: 11px; font-weight: 600;
}
#detail .edge-section { margin-top: 16px; }
#detail .edge-section h4 { font-size: 12px; color: #8b949e; margin-bottom: 6px; text-transform: uppercase; }
#detail .edge-item {
  padding: 6px 8px; margin-bottom: 4px; background: #0d1117; border-radius: 4px;
  font-size: 12px; cursor: pointer; display: flex; justify-content: space-between; align-items: center;
}
#detail .edge-item:hover { background: #1c2128; }
#detail .edge-type { font-size: 10px; padding: 1px 6px; border-radius: 8px; font-weight: 600; }

/* Bottom bar */
#bottombar {
  display: flex; align-items: center; gap: 8px; padding: 6px 16px;
  background: #161b22; border-top: 1px solid #30363d; height: 36px;
}
#bottombar button {
  background: #21262d; border: 1px solid #30363d; color: #c9d1d9; border-radius: 4px;
  padding: 3px 10px; font-size: 12px; cursor: pointer;
}
#bottombar button:hover { background: #30363d; }
.legend { display: flex; gap: 10px; margin-left: 16px; font-size: 11px; color: #8b949e; }
.legend-item { display: flex; align-items: center; gap: 4px; }
.legend-dot { width: 8px; height: 8px; border-radius: 50%; display: inline-block; }

/* Loading overlay */
#loading {
  position: absolute; top: 0; left: 0; right: 0; bottom: 0;
  display: flex; align-items: center; justify-content: center;
  background: rgba(13,17,23,0.85); z-index: 20; font-size: 16px; color: #8b949e;
}
#loading.hidden { display: none; }
.spinner { width: 24px; height: 24px; border: 3px solid #30363d; border-top-color: #58a6ff; border-radius: 50%; animation: spin 0.8s linear infinite; margin-right: 12px; }
@keyframes spin { to { transform: rotate(360deg); } }
</style>
</head>
<body>

<div id="topbar">
  <div class="logo">MIMIR</div>
  <select id="repo-select"><option value="">Loading...</option></select>
  <input id="search" type="text" placeholder="Search symbols..." />
  <div id="stats">
    <span id="stat-nodes">0 nodes</span>
    <span id="stat-edges">0 edges</span>
    <span id="stat-clusters">0 clusters</span>
  </div>
</div>

<div id="main">
  <div id="graph-container">
    <div id="sigma-container"></div>
    <div id="loading"><div class="spinner"></div>Loading graph...</div>
  </div>
  <div id="detail">
    <h3>
      <span id="detail-title">Symbol</span>
      <button class="close-btn" onclick="closeDetail()">&times;</button>
    </h3>
    <div class="field"><label>Name</label><div class="val" id="d-name"></div></div>
    <div class="field"><label>Kind</label><div class="val" id="d-kind"></div></div>
    <div class="field"><label>File</label><div class="val" id="d-file"></div></div>
    <div class="field"><label>Lines</label><div class="val" id="d-lines"></div></div>
    <div class="field"><label>Cluster</label><div class="val" id="d-cluster"></div></div>
    <div class="edge-section">
      <h4>Incoming Edges</h4>
      <div id="d-incoming"></div>
    </div>
    <div class="edge-section">
      <h4>Outgoing Edges</h4>
      <div id="d-outgoing"></div>
    </div>
  </div>
</div>

<div id="bottombar">
  <button onclick="fitGraph()">Fit</button>
  <button onclick="zoomIn()">+</button>
  <button onclick="zoomOut()">&minus;</button>
  <div class="legend">
    <div class="legend-item"><span class="legend-dot" style="background:#58a6ff"></span>Function</div>
    <div class="legend-item"><span class="legend-dot" style="background:#3fb950"></span>Class</div>
    <div class="legend-item"><span class="legend-dot" style="background:#39d2c0"></span>Method</div>
    <div class="legend-item"><span class="legend-dot" style="background:#bc8cff"></span>Interface</div>
    <div class="legend-item"><span class="legend-dot" style="background:#d29922"></span>Variable</div>
    <div class="legend-item"><span class="legend-dot" style="background:#8b949e"></span>Type</div>
  </div>
</div>

<script src="https://cdnjs.cloudflare.com/ajax/libs/graphology/0.25.4/graphology.umd.min.js"></script>
<script src="https://cdnjs.cloudflare.com/ajax/libs/sigma.js/2.4.0/sigma.min.js"></script>
<script src="https://cdn.jsdelivr.net/npm/graphology-layout-forceatlas2@0.10.1/build/graphology-layout-forceatlas2.min.js"></script>

<script>
const KIND_COLORS = {
  Function: "#58a6ff", Class: "#3fb950", Method: "#39d2c0",
  Interface: "#bc8cff", Variable: "#d29922", Constant: "#d29922",
  Type: "#8b949e"
};
const EDGE_COLORS = {
  CALLS: "#484f58", IMPORTS: "#1f6feb", EXTENDS: "#238636",
  IMPLEMENTS: "#8957e5", MEMBER_OF: "#9e6a03"
};

let sigma = null;
let graph = null;
let fa2Layout = null;
let allNodes = [];
let nodeMap = {};
let currentRepo = "";

async function fetchJSON(url) {
  const r = await fetch(url);
  return r.json();
}

async function loadRepos() {
  const data = await fetchJSON("/repos");
  const sel = document.getElementById("repo-select");
  sel.innerHTML = "";
  if (!data.repos || data.repos.length === 0) {
    sel.innerHTML = '<option value="">No repos indexed</option>';
    return;
  }
  data.repos.forEach(repo => {
    const o = document.createElement("option");
    o.value = repo.name; o.textContent = repo.name;
    sel.appendChild(o);
  });
  sel.addEventListener("change", () => loadGraph(sel.value));
  loadGraph(data.repos[0].name);
}

async function loadGraph(repoName) {
  if (!repoName) return;
  currentRepo = repoName;
  const loading = document.getElementById("loading");
  loading.classList.remove("hidden");

  if (sigma) { sigma.kill(); sigma = null; }
  graph = new graphology.Graph({ type: "directed", multi: true });

  const data = await fetchJSON("/repo/" + encodeURIComponent(repoName) + "/graph");
  const nodes = data.nodes || [];
  const edges = data.edges || [];
  allNodes = nodes;
  nodeMap = {};

  const degreeCount = {};
  edges.forEach(e => {
    degreeCount[e.FromUID] = (degreeCount[e.FromUID] || 0) + 1;
    degreeCount[e.ToUID] = (degreeCount[e.ToUID] || 0) + 1;
  });
  const maxDeg = Math.max(1, ...Object.values(degreeCount));

  nodes.forEach(n => {
    nodeMap[n.UID] = n;
    const deg = degreeCount[n.UID] || 0;
    const size = 3 + (deg / maxDeg) * 12;
    graph.addNode(n.UID, {
      label: n.Name,
      x: Math.random() * 100 - 50,
      y: Math.random() * 100 - 50,
      size: size,
      color: KIND_COLORS[n.Kind] || "#8b949e",
      kind: n.Kind,
      filePath: n.FilePath,
      startLine: n.StartLine,
      endLine: n.EndLine,
      clusterID: n.ClusterID,
      originalColor: KIND_COLORS[n.Kind] || "#8b949e"
    });
  });

  edges.forEach(e => {
    if (graph.hasNode(e.FromUID) && graph.hasNode(e.ToUID)) {
      try {
        graph.addEdge(e.FromUID, e.ToUID, {
          color: (EDGE_COLORS[e.Type] || "#484f58") + alphaHex(e.Confidence),
          size: 0.5 + e.Confidence * 1.5,
          type: "arrow",
          edgeType: e.Type
        });
      } catch(ex) {}
    }
  });

  sigma = new Sigma(graph, document.getElementById("sigma-container"), {
    renderEdgeLabels: false,
    defaultEdgeType: "arrow",
    labelSize: 11,
    labelColor: { color: "#c9d1d9" },
    labelFont: "-apple-system, sans-serif",
    labelRenderedSizeThreshold: 6,
    edgeLabelSize: 10,
    stagePadding: 40,
    nodeReducer: (node, data) => {
      const res = { ...data };
      if (searchQuery && !data.label.toLowerCase().includes(searchQuery)) {
        res.color = "#21262d";
        res.label = "";
        res.zIndex = 0;
      } else if (searchQuery) {
        res.zIndex = 1;
        res.highlighted = true;
      }
      if (hoveredNode && hoveredNode !== node && !hoveredNeighbors.has(node)) {
        res.color = "#21262d";
        res.label = "";
        res.zIndex = 0;
      }
      if (hoveredNode === node) {
        res.highlighted = true;
        res.zIndex = 2;
      }
      return res;
    },
    edgeReducer: (edge, data) => {
      const res = { ...data };
      if (hoveredNode) {
        const src = graph.source(edge);
        const tgt = graph.target(edge);
        if (src !== hoveredNode && tgt !== hoveredNode) {
          res.hidden = true;
        } else {
          res.color = "#58a6ff";
          res.size = 2;
        }
      }
      return res;
    }
  });

  sigma.on("clickNode", ({ node }) => showDetail(node));
  sigma.on("enterNode", ({ node }) => { hoveredNode = node; hoveredNeighbors = new Set(graph.neighbors(node)); sigma.refresh(); });
  sigma.on("leaveNode", () => { hoveredNode = null; hoveredNeighbors = new Set(); sigma.refresh(); });
  sigma.on("clickStage", () => closeDetail());

  // Run ForceAtlas2
  if (typeof ForceAtlas2Layout !== "undefined" || typeof graphologyLayoutForceAtlas2 !== "undefined") {
    const fa2Mod = window.graphologyLayoutForceAtlas2 || window.ForceAtlas2Layout;
    if (fa2Mod && fa2Mod.default) {
      const fa2 = fa2Mod.default;
      fa2.assign(graph, { iterations: 100, settings: fa2.inferSettings(graph) });
    } else if (fa2Mod) {
      fa2Mod.assign(graph, { iterations: 100, settings: fa2Mod.inferSettings(graph) });
    }
  }

  // Update cluster count
  const clusterSet = new Set();
  nodes.forEach(n => { if (n.ClusterID) clusterSet.add(n.ClusterID); });

  document.getElementById("stat-nodes").textContent = nodes.length + " nodes";
  document.getElementById("stat-edges").textContent = edges.length + " edges";
  document.getElementById("stat-clusters").textContent = clusterSet.size + " clusters";

  loading.classList.add("hidden");
  setTimeout(() => fitGraph(), 100);
}

function alphaHex(confidence) {
  const a = Math.round(Math.max(0.2, Math.min(1, confidence)) * 255);
  return a.toString(16).padStart(2, "0");
}

let searchQuery = "";
let hoveredNode = null;
let hoveredNeighbors = new Set();

document.getElementById("search").addEventListener("input", (e) => {
  searchQuery = e.target.value.toLowerCase();
  if (sigma) sigma.refresh();
});

async function showDetail(nodeId) {
  const n = nodeMap[nodeId];
  if (!n) return;
  document.getElementById("d-name").textContent = n.Name;
  const kindEl = document.getElementById("d-kind");
  kindEl.innerHTML = '<span class="kind-badge" style="background:' + (KIND_COLORS[n.Kind] || "#8b949e") + '22; color:' + (KIND_COLORS[n.Kind] || "#8b949e") + '">' + n.Kind + '</span>';
  document.getElementById("d-file").textContent = n.FilePath;
  document.getElementById("d-lines").textContent = n.StartLine + " - " + n.EndLine;
  document.getElementById("d-cluster").textContent = n.ClusterID || "—";
  document.getElementById("detail-title").textContent = n.Name;
  document.getElementById("detail").classList.add("open");

  // Use local graph edges (already loaded) for immediate display
  const incoming = [];
  const outgoing = [];
  graph.forEachEdge(nodeId, (edge, attrs, src, tgt) => {
    if (tgt === nodeId) incoming.push({ FromUID: src, ToUID: tgt, Type: attrs.edgeType || "CALLS", Confidence: 1 });
    if (src === nodeId) outgoing.push({ FromUID: src, ToUID: tgt, Type: attrs.edgeType || "CALLS", Confidence: 1 });
  });
  renderEdges("d-incoming", incoming, true);
  renderEdges("d-outgoing", outgoing, false);
}

function renderEdges(containerId, edges, isIncoming) {
  const el = document.getElementById(containerId);
  if (!edges || edges.length === 0) {
    el.innerHTML = '<div style="color:#484f58;font-size:12px">None</div>';
    return;
  }
  el.innerHTML = edges.map(e => {
    const uid = isIncoming ? e.FromUID : e.ToUID;
    const peer = nodeMap[uid];
    const name = peer ? peer.Name : uid.substring(0, 8);
    const col = EDGE_COLORS[e.Type] || "#484f58";
    return '<div class="edge-item" onclick="focusNode(\'' + uid + '\')">' +
      '<span>' + escapeHtml(name) + '</span>' +
      '<span class="edge-type" style="background:' + col + '33;color:' + col + '">' + e.Type + '</span>' +
    '</div>';
  }).join("");
}

function escapeHtml(s) {
  const d = document.createElement("div");
  d.textContent = s;
  return d.innerHTML;
}

function focusNode(uid) {
  if (!sigma || !graph.hasNode(uid)) return;
  const attrs = graph.getNodeAttributes(uid);
  sigma.getCamera().animate({ x: attrs.x, y: attrs.y, ratio: 0.3 }, { duration: 300 });
  showDetail(uid);
}

function closeDetail() {
  document.getElementById("detail").classList.remove("open");
}

function fitGraph() {
  if (sigma) {
    sigma.getCamera().animate({ x: 0.5, y: 0.5, ratio: 1 }, { duration: 300 });
  }
}

function zoomIn() {
  if (sigma) {
    const cam = sigma.getCamera();
    cam.animate({ ratio: cam.ratio / 1.5 }, { duration: 200 });
  }
}

function zoomOut() {
  if (sigma) {
    const cam = sigma.getCamera();
    cam.animate({ ratio: cam.ratio * 1.5 }, { duration: 200 });
  }
}

loadRepos();
</script>
</body>
</html>`
