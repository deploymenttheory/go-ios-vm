// One neighborhood is shown at a time. Moving to a node fetches its bounded
// neighborhood; the browser never assembles an unbounded client-side graph.
const graphEdgeKinds = [
  "belongs_to_artifact", "contains_target", "declares_component",
  "contains_device_tree_node", "declares_driver", "observed_apfs_path",
  "has_finding", "has_property", "declares_driver_dependency",
  "declared_by_driver", "resolves_to_apfs_object", "records_observation",
  "has_revision", "belongs_to_finding", "supersedes_revision", "supporting_observation",
  "contradicting_observation", "supporting_evidence", "contradicting_evidence",
  "supporting_revision", "contradicting_revision", "assessed_as",
  "assesses_revision", "retains_evidence", "used_by_revision",
  "supports_observation",
];
const graphView = {
  target: "", targetRecord: null, neighborhood: null, selected: null,
  search: "", sort: "kind", descending: false, request: 0,
};

function graphLabel(value) { return String(value || "").replaceAll("_", " "); }
function graphRoute(changes) {
  const url = new URL(window.location.href);
  for (const [key, value] of Object.entries(changes)) {
    if (value === "" || value === null || value === undefined) url.searchParams.delete(key);
    else url.searchParams.set(key, String(value));
  }
  url.searchParams.set("screen", "graph");
  return url;
}
function graphSemantic(edge, nodes) {
  if (edge.kind === "declares_driver_dependency" || edge.kind === "declared_by_driver") return "declared";
  const revision = [nodes.get(edge.from), nodes.get(edge.to)].find((item) => item && item.kind === "revision" && /^(observed|inferred|unknown) · /.test(item.label));
  if (revision) return revision.label.split(" · ")[0];
  if (edge.basis.origin === "stored_extraction") return "observed";
  return "stored";
}
function graphNodes(data) {
  return new Map([data.center, ...(data.nodes || [])].map((item) => [item.id, item]));
}
function graphFilteredEdges(data) {
  const edges = data.edges || [];
  const needle = graphView.search.trim().toLocaleLowerCase();
  if (!needle) return edges;
  const nodes = graphNodes(data);
  return edges.filter((edge) => [edge.kind, nodes.get(edge.from)?.label, nodes.get(edge.to)?.label,
    nodes.get(edge.from)?.kind, nodes.get(edge.to)?.kind].some((value) => String(value || "").toLocaleLowerCase().includes(needle)));
}
function graphSortedEdges(edges, nodes) {
  const copy = [...edges];
  const key = graphView.sort;
  const textFor = (edge) => {
    if (key === "from" || key === "to") return nodes.get(edge[key])?.label || "";
    if (key === "basis") return `${edge.basis.record_kind} ${edge.basis.record_id}`;
    return edge.kind;
  };
  copy.sort((a, b) => textFor(a).localeCompare(textFor(b)) || a.id.localeCompare(b.id));
  if (graphView.descending) copy.reverse();
  return copy;
}
function graphDetailLink(item, data, target) {
  const basis = data.edges.find((edge) => (edge.from === item.id || edge.to === item.id) &&
    edge.basis.record_kind === item.kind)?.basis;
  const recordID = item.record_id || basis?.record_id;
  if (!recordID) return null;
  if (item.kind === "finding") return withRoute({ screen: "", target, finding: recordID, observation: "", evidence: "", view: "findings" });
  if (item.kind === "observation") return withRoute({ screen: "", target, observation: recordID, finding: "", evidence: "", view: "observations" });
  return null;
}
function graphBuildShell() {
  const host = byId("graph-screen");
  clear(host);
  host.hidden = false;
  host.append(node("p", "eyebrow", "Relationship map"));
  host.append(node("h2", "", "Follow retained relationships"));
  host.append(node("p", "hint", "Select a node or relationship to inspect its basis. Expand a node to request its own bounded neighborhood."));
  const controls = node("div", "graph-controls");
  const componentLabel = node("label", "", "Component");
  const component = node("select"); component.id = "graph-component";
  component.append(node("option", "", "All components")); component.firstChild.value = "";
  componentLabel.append(component);
  const kindLabel = node("label", "", "Relationship type");
  const kind = node("select"); kind.id = "graph-edge-kind";
  kind.append(node("option", "", "All types")); kind.firstChild.value = "";
  for (const value of graphEdgeKinds) {
    const option = node("option", "", graphLabel(value)); option.value = value; kind.append(option);
  }
  kindLabel.append(kind);
  const limitLabel = node("label", "", "Maximum relationships");
  const limit = node("select"); limit.id = "graph-limit";
  for (const value of [25, 50, 100]) { const option = node("option", "", String(value)); option.value = String(value); limit.append(option); }
  limitLabel.append(limit);
  const searchLabel = node("label", "", "Search this neighborhood");
  const search = node("input"); search.id = "graph-search"; search.type = "search"; search.placeholder = "Node or relationship";
  searchLabel.append(search);
  add(controls, componentLabel, kindLabel, limitLabel, searchLabel);
  host.append(controls);
  const componentWarning = node("p", "hint"); componentWarning.id = "graph-component-warning";
  componentWarning.hidden = true; host.append(componentWarning);
  const reset = node("button", "button", "Start at target"); reset.id = "graph-reset"; reset.type = "button";
  host.append(reset);
  const scope = node("p", "graph-scope hint"); scope.id = "graph-scope"; scope.setAttribute("role", "status"); host.append(scope);
  const gaps = node("div", "graph-gaps"); gaps.id = "graph-gaps"; host.append(gaps);
  const truncated = node("div", "notice graph-truncated"); truncated.id = "graph-truncated"; truncated.hidden = true; host.append(truncated);
  const legend = node("div", "graph-legend"); legend.setAttribute("aria-label", "Relationship legend");
  for (const [semantic, description] of [
    ["observed", "Observed extraction"], ["inferred", "Inferred finding"],
    ["unknown", "Unknown finding"], ["declared", "Declared dependency"],
    ["stored", "Stored link"], ["unsupported", "Unsupported coverage"],
  ]) {
    const item = node("span", "graph-legend-item", description); item.dataset.semantic = semantic; legend.append(item);
  }
  host.append(legend);
  const columns = node("div", "graph-columns");
  const left = node("div", "graph-main");
  const mapTitle = node("h3", "", "Relationship map");
  const map = node("div", "graph-map"); map.id = "graph-map"; map.setAttribute("aria-label", "Directed relationship map");
  const tableTitle = node("h3", "", "Relationship table");
  const tableHelp = node("p", "hint", "The sortable table contains the same relationships as the map and supports keyboard selection.");
  const table = node("div", "graph-table-wrap"); table.id = "graph-table-wrap";
  add(left, mapTitle, map, tableTitle, tableHelp, table);
  const detail = node("aside", "panel graph-detail"); detail.id = "graph-detail";
  detail.setAttribute("aria-label", "Selected relationship detail"); detail.setAttribute("aria-live", "polite");
  add(columns, left, detail); host.append(columns);
  component.addEventListener("change", () => {
    graphNavigate({ graph_node: component.value });
  });
  kind.addEventListener("change", () => graphNavigate({ graph_edge: kind.value }));
  limit.addEventListener("change", () => graphNavigate({ graph_limit: limit.value }));
  search.addEventListener("input", () => { graphView.search = search.value; graphRender(); });
  reset.addEventListener("click", () => graphNavigate({ graph_node: "", graph_edge: "" }));
}
function graphSelect(type, id) {
  graphView.selected = { type, id };
  graphRenderSelection();
}
function graphNodeButton(item, className = "") {
  const button = node("button", `graph-node ${className}`);
  button.type = "button";
  button.dataset.nodeId = item.id;
  button.dataset.kind = item.kind;
  button.textContent = item.label;
  button.title = `${graphLabel(item.kind)}: ${item.label}`;
  button.setAttribute("aria-label", `Select ${graphLabel(item.kind)} ${item.label}`);
  button.addEventListener("click", () => graphSelect("node", item.id));
  return button;
}
function graphRenderMap(edges, nodes) {
  const map = byId("graph-map"); clear(map);
  if (!edges.length) { message(map, graphView.search ? "No matching relationships in this loaded neighborhood." : "This neighborhood has no relationships of the selected type."); return; }
  const star = node("div", "graph-star");
  const hub = node("div", "graph-hub");
  hub.append(node("span", "graph-hub-label", "Selected center"), graphNodeButton(graphView.neighborhood.center, "graph-center-node"));
  const spokes = node("div", "graph-spokes");
  for (const edge of edges) {
    const lane = node("div", "graph-lane"); lane.dataset.edgeId = edge.id;
    lane.dataset.semantic = graphSemantic(edge, nodes);
    const outgoing = edge.from === graphView.neighborhood.center.id;
    const neighbor = nodes.get(outgoing ? edge.to : edge.from);
    lane.dataset.direction = outgoing ? "out" : "in";
    const relation = node("button", "graph-relation", `${outgoing ? "" : "← "}${graphLabel(edge.kind)}${outgoing ? " →" : ""}`);
    relation.type = "button"; relation.setAttribute("aria-label", `Select relationship ${graphLabel(edge.kind)}`);
    relation.addEventListener("click", () => graphSelect("edge", edge.id));
    add(lane, relation, graphNodeButton(neighbor));
    spokes.append(lane);
  }
  add(star, hub, spokes); map.append(star);
}
function graphRenderTable(edges, nodes) {
  const wrap = byId("graph-table-wrap"); clear(wrap);
  const table = node("table", "graph-table");
  const caption = node("caption", "", `${edges.length} loaded relationships for this neighborhood`); table.append(caption);
  const thead = node("thead"); const header = node("tr");
  for (const [key, label] of [["from", "From"], ["kind", "Relationship"], ["to", "To"], ["basis", "Basis record"]]) {
    const th = node("th"); th.scope = "col";
    th.setAttribute("aria-sort", graphView.sort === key ? (graphView.descending ? "descending" : "ascending") : "none");
    const button = node("button", "graph-sort", label); button.type = "button";
    button.dataset.sort = graphView.sort === key ? (graphView.descending ? "descending" : "ascending") : "none";
    button.addEventListener("click", () => {
      graphView.descending = graphView.sort === key ? !graphView.descending : false;
      graphView.sort = key; graphRenderTable(edges, nodes); graphRenderSelection();
    });
    th.append(button); header.append(th);
  }
  thead.append(header); table.append(thead);
  const tbody = node("tbody");
  for (const edge of graphSortedEdges(edges, nodes)) {
    const row = node("tr"); row.dataset.edgeId = edge.id;
    const from = node("td", "", nodes.get(edge.from).label);
    const relation = node("td");
    const select = node("button", "graph-table-select", graphLabel(edge.kind)); select.type = "button";
    select.addEventListener("click", () => graphSelect("edge", edge.id)); relation.append(select);
    add(row, from, relation, node("td", "", nodes.get(edge.to).label),
      node("td", "", `${edge.basis.record_kind}: ${edge.basis.record_id}`));
    tbody.append(row);
  }
  table.append(tbody); wrap.append(table);
}
function graphRenderSelection() {
  const data = graphView.neighborhood;
  if (!data) return;
  const nodes = graphNodes(data);
  const detail = byId("graph-detail"); clear(detail);
  let selected = graphView.selected;
  if (!selected || (selected.type === "node" && !nodes.has(selected.id)) ||
      (selected.type === "edge" && !data.edges.some((item) => item.id === selected.id))) {
    selected = { type: "node", id: data.center.id }; graphView.selected = selected;
  }
  for (const element of document.querySelectorAll("[data-edge-id], [data-node-id]")) {
    element.classList.toggle("is-selected", element.dataset.edgeId === selected.id || element.dataset.nodeId === selected.id);
  }
  if (selected.type === "node") {
    const item = nodes.get(selected.id);
    detail.append(node("p", "eyebrow", graphLabel(item.kind)), node("h3", "", item.label));
    detail.append(metadata([["Target ID", item.target_id], ["Node ID", item.id], ["State", item.state || "Not recorded"]]));
    const link = graphDetailLink(item, data, graphView.target);
    if (link) { const a = node("a", "button", `Open ${item.kind} detail`); a.href = link; detail.append(a); }
    if (item.id !== data.center.id) {
      const expand = node("button", "button button-primary", "Expand this node"); expand.type = "button";
      expand.addEventListener("click", () => graphNavigate({ graph_node: item.id })); detail.append(expand);
    }
    if (item.kind === "revision") {
      detail.append(node("p", "hint", "The revision label records its basis and presence. Expand it to inspect supporting, contradicting, and assessment links."));
    }
    return;
  }
  const edge = data.edges.find((item) => item.id === selected.id);
  detail.append(node("p", "eyebrow", "Directed relationship"), node("h3", "", graphLabel(edge.kind)));
  detail.append(node("p", "graph-direction", `${nodes.get(edge.from).label} → ${nodes.get(edge.to).label}`));
  const semantic = graphSemantic(edge, nodes);
  detail.append(pill(semantic, semantic === "declared" ? "Declared, runtime unverified" : graphLabel(semantic)));
  detail.append(metadata([
    ["Record", `${edge.basis.record_kind}: ${edge.basis.record_id}`],
    ["Origin", graphLabel(edge.basis.origin)], ["Extractor", edge.basis.extractor || "Not recorded"],
    ["Run IDs", (edge.basis.run_ids || []).join(", ") || "Not recorded"],
    ["Coverage", edge.basis.coverage_state || "See coverage gaps"],
    ["Relationship ID", edge.id],
  ]));
  if (edge.basis.runs_truncated) detail.append(node("p", "hint", "More run associations exist than are shown here."));
  if (edge.basis.source) { detail.append(node("h4", "", "Logical source"), sourceChain(edge.basis.source)); }
  if (edge.basis.record_kind === "observation") {
    const a = node("a", "button", "Open observation and retained evidence");
    a.href = withRoute({ screen: "", target: graphView.target, observation: edge.basis.record_id,
      finding: "", evidence: "", view: "observations" }); detail.append(a);
  } else if (edge.basis.record_kind === "finding") {
    const a = node("a", "button", "Open finding detail");
    a.href = withRoute({ screen: "", target: graphView.target, finding: edge.basis.record_id,
      observation: "", evidence: "", view: "findings" }); detail.append(a);
  }
}
function graphRender() {
  const data = graphView.neighborhood;
  if (!data) return;
  const nodes = graphNodes(data);
  const edges = graphFilteredEdges(data);
  byId("graph-scope").textContent = `${graphLabel(data.center.kind)}: ${data.center.label} · ${data.edges.length} loaded relationship${data.edges.length === 1 ? "" : "s"}${graphView.search ? ` · ${edges.length} search matches` : ""}${data.run_id ? ` · latest run ${data.run_id}` : ""}`;
  const gaps = byId("graph-gaps"); clear(gaps);
  if (!data.run_id) {
    const unknown = node("div", "graph-gap", "No retained inspection run for this target; absence cannot be inferred.");
    unknown.dataset.state = "unexamined"; gaps.append(unknown);
  }
  for (const gap of data.gaps || []) {
    const item = node("div", "graph-gap", `${gap.component}: ${gap.state}${gap.reason ? ` · ${gap.reason}` : ""}`);
    item.dataset.state = gap.state; gaps.append(item);
  }
  const truncated = byId("graph-truncated"); truncated.hidden = !data.truncated;
  if (data.truncated) truncated.textContent = "Neighborhood limit reached. Unshown relationships may exist. Choose a relationship type or expand a specific node; no absence is implied.";
  graphRenderMap(edges, nodes); graphRenderTable(edges, nodes); graphRenderSelection();
}
async function graphLoad() {
  const current = new URL(window.location.href);
  const nodeID = current.searchParams.get("graph_node") || "";
  const edgeKind = current.searchParams.get("graph_edge") || "";
  const limit = Number(current.searchParams.get("graph_limit") || "25");
  byId("graph-edge-kind").value = edgeKind;
  byId("graph-limit").value = [25, 50, 100].includes(limit) ? String(limit) : "25";
  byId("graph-component").value = [...byId("graph-component").options].some((option) => option.value === nodeID) ? nodeID : "";
  const request = ++graphView.request;
  message(byId("graph-map"), "Loading one bounded neighborhood…");
  try {
    const data = await api(queryURL("/api/v1/graph", {
      target_id: graphView.target, node_id: nodeID, edge_kind: edgeKind, limit: [25, 50, 100].includes(limit) ? limit : 25,
    }));
    if (request !== graphView.request) return;
    const nodes = graphNodes(data);
    if (data.center.target_id !== graphView.target ||
        data.nodes.some((item) => item.target_id !== graphView.target) ||
        data.edges.some((item) => !nodes.has(item.from) || !nodes.has(item.to))) {
      throw new Error("Graph response does not match the selected target.");
    }
    graphView.neighborhood = data; graphView.selected = { type: "node", id: data.center.id };
    graphRender();
  } catch (error) {
    if (request !== graphView.request) return;
    graphView.neighborhood = null; message(byId("graph-map"), error.message, true);
    clear(byId("graph-table-wrap")); clear(byId("graph-detail"));
  }
}
function graphNavigate(changes) {
  const url = graphRoute(changes);
  window.history.pushState(null, "", url);
  graphLoad();
}
async function startGraph() {
  graphView.target = selectedTargetID;
  graphBuildShell();
  if (!selectedTargetID) {
    message(byId("graph-map"), "Choose an exact firmware target to review its relationships.");
    byId("graph-component").disabled = true;
    byId("graph-edge-kind").disabled = true;
    byId("graph-limit").disabled = true;
    return;
  }
  try {
    const record = await api(`/api/v1/targets/${encodeURIComponent(selectedTargetID)}`);
    if (record.target.id !== selectedTargetID) throw new Error("Selected target identity changed.");
    graphView.targetRecord = record; showTarget(record);
    const components = await api(queryURL("/api/v1/graph", {
      target_id: graphView.target, edge_kind: "declares_component", limit: 100,
    }));
    for (const item of components.nodes.filter((entry) => entry.kind === "component").sort((a, b) => a.label.localeCompare(b.label))) {
      const option = node("option", "", item.label); option.value = item.id;
      byId("graph-component").append(option);
    }
    if (components.truncated) {
      byId("graph-component-warning").hidden = false;
      byId("graph-component-warning").textContent = "Component choices reached the 100-link limit. Unlisted components may exist; no absence is implied.";
    }
    await graphLoad();
  } catch (error) { message(byId("graph-map"), error.message, true); }
  window.addEventListener("popstate", () => graphLoad());
}
