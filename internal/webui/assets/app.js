// Local, read-only browser for the versioned inspection API. All retained
// strings are assigned as text; no API value is interpreted as markup.
const byId = (id) => document.getElementById(id);
const route = new URLSearchParams(window.location.search);
const selectedTargetID = route.get("target") || "";
const selectedRunID = route.get("run") || "";
const view = route.get("view") === "observations" || route.has("observation") ? "observations" : "findings";
const screen = route.get("screen") === "monitor" ? "monitor" : "browse";

class ExactInteger {
  constructor(text) { this.text = text; }
  toString() { return this.text; }
}

// JSON.parse rounds uint64 values before a reviver can see them. Wrap only
// integers outside JavaScript's exact range while scanning outside strings.
function parseExactJSON(source) {
  const marker = `__exact_integer_${Math.random().toString(36).slice(2)}__`;
  const max = BigInt(Number.MAX_SAFE_INTEGER);
  let quoted = false;
  let escaped = false;
  let rewritten = "";
  for (let i = 0; i < source.length; i += 1) {
    const char = source[i];
    if (quoted) {
      rewritten += char;
      if (escaped) escaped = false;
      else if (char === "\\") escaped = true;
      else if (char === '"') quoted = false;
      continue;
    }
    if (char === '"') {
      quoted = true;
      rewritten += char;
      continue;
    }
    if (char === "-" || (char >= "0" && char <= "9")) {
      const token = source.slice(i).match(/^-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?/);
      if (token) {
        const number = token[0];
        if (/^-?\d+$/.test(number) && (BigInt(number) > max || BigInt(number) < -max)) {
          rewritten += `{"${marker}":"${number}"}`;
        } else {
          rewritten += number;
        }
        i += number.length - 1;
        continue;
      }
    }
    rewritten += char;
  }
  return JSON.parse(rewritten, (_key, value) => {
    if (value && typeof value === "object" && Object.keys(value).length === 1 &&
        Object.prototype.hasOwnProperty.call(value, marker)) {
      return new ExactInteger(value[marker]);
    }
    return value;
  });
}

function formatExactJSON(value, depth = 0) {
  if (value instanceof ExactInteger) return value.text;
  if (value === null) return "null";
  if (typeof value === "string") return JSON.stringify(value);
  if (typeof value !== "object") return String(value);
  const indent = "  ".repeat(depth + 1);
  const closing = "  ".repeat(depth);
  if (Array.isArray(value)) {
    if (!value.length) return "[]";
    return `[\n${value.map((item) => `${indent}${formatExactJSON(item, depth + 1)}`).join(",\n")}\n${closing}]`;
  }
  const entries = Object.entries(value);
  if (!entries.length) return "{}";
  return `{\n${entries.map(([key, item]) => `${indent}${JSON.stringify(key)}: ${formatExactJSON(item, depth + 1)}`).join(",\n")}\n${closing}}`;
}

function node(tag, className = "", text = "") {
  const element = document.createElement(tag);
  if (className) element.className = className;
  if (text !== "") element.textContent = String(text);
  return element;
}
function clear(element) { element.replaceChildren(); }
function add(parent, ...children) { parent.append(...children); return parent; }
function message(parent, text, error = false) {
  clear(parent);
  const block = node("div", "message", text);
  if (error) block.setAttribute("role", "alert");
  parent.append(block);
}
function safeLabel(value, fallback = "Unknown") { return value === null || value === undefined || value === "" ? fallback : String(value); }
function shortID(value) { return value ? `${String(value).slice(0, 12)}…` : "Unknown"; }
function dateLabel(value) {
  if (!value) return "Not recorded";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? String(value) : date.toLocaleString();
}
function tone(value) {
  if (["complete", "present", "accepted", "verified"].includes(value)) return "good";
  if (["failed", "cancelled", "rejected"].includes(value)) return "bad";
  if (["partial", "interrupted", "disputed", "stale"].includes(value)) return "warn";
  if (["unknown", "unexamined"].includes(value)) return "unknown";
  return "muted";
}
function pill(value, label = value) {
  const item = node("span", "pill", safeLabel(label));
  item.dataset.tone = tone(value);
  return item;
}
function metadata(entries, className = "metadata") {
  const list = node("dl", className);
  for (const [label, value] of entries) {
    add(list, node("dt", "", label), node("dd", "", safeLabel(value)));
  }
  return list;
}
function withRoute(changes, removed = []) {
  const next = new URLSearchParams(route);
  for (const key of removed) next.delete(key);
  for (const [key, value] of Object.entries(changes)) {
    if (value === "" || value === null || value === undefined) next.delete(key);
    else next.set(key, String(value));
  }
  const query = next.toString();
  return query ? `/?${query}` : "/";
}
function anchor(text, changes, removed = [], className = "") {
  const link = node("a", className, text);
  link.href = withRoute(changes, removed);
  return link;
}
function setFormValue(id, key) { byId(id).value = route.get(key) || ""; }
function navigateForm(event, changes, removed = []) {
  event.preventDefault();
  window.location.assign(withRoute(changes, removed));
}
function queryURL(path, values) {
  const query = new URLSearchParams();
  for (const [key, value] of Object.entries(values)) {
    if (value !== "" && value !== null && value !== undefined) query.set(key, String(value));
  }
  return `${path}?${query.toString()}`;
}
async function api(path) {
  const response = await fetch(path, { cache: "no-store", headers: { Accept: "application/json" } });
  const body = parseExactJSON(await response.text());
  if (!response.ok || body.state !== "ready") {
    const safe = body.diagnostic && body.diagnostic.message;
    throw new Error(safe || "The evidence request could not be completed.");
  }
  return body.data;
}
async function checkHealth() {
  try {
    const response = await fetch("/api/v1/health", { cache: "no-store" });
    const health = parseExactJSON(await response.text());
    if (response.ok && health.state === "ready") {
      byId("status").textContent = "Evidence database connected";
      byId("status-dot").dataset.state = "ready";
      return;
    }
  } catch (_) { /* A stopped local server is reported below. */ }
  byId("status").textContent = "Evidence database unavailable";
  byId("status-dot").dataset.state = "unavailable";
}
function renderPages(element, cursorKey, nextCursor) {
  clear(element);
  const current = route.get(cursorKey) || "";
  if (!current && !nextCursor) { element.hidden = true; return; }
  element.hidden = false;
  if (current) element.append(anchor("First page", { [cursorKey]: "" }, [], "button"));
  if (nextCursor) element.append(anchor("Next page", { [cursorKey]: nextCursor }, [], "button"));
}
function targetTitle(target) {
  return `${(target.products || []).join(", ") || "Unknown product"} · ${safeLabel(target.build, "Unknown build")}`;
}
function targetCard(record) {
  const target = record.target;
  const card = node("article", `target-card${target.id === selectedTargetID ? " selected" : ""}`);
  const link = anchor("", { target: target.id, run: "", finding: "", observation: "", evidence: "", run_cursor: "", result_cursor: "" }, [], "card-link");
  add(link, node("div", "card-title", targetTitle(target)), node("div", "card-subtitle", `Identity ${target.identity} · ${safeLabel(target.board, "Board unknown")} · iOS ${safeLabel(target.version)}`));
  card.append(link);
  return card;
}
async function loadTargets() {
  const area = byId("target-results");
  message(area, "Loading targets…");
  try {
    const page = await api(queryURL("/api/v1/targets", {
      product: route.get("product"), build: route.get("build"), identity: route.get("identity"),
      cursor: route.get("target_cursor"), limit: 25,
    }));
    clear(area);
    if (!page.items.length) message(area, "No retained targets match this exact search. Try another product, build, or identity.");
    else for (const item of page.items) area.append(targetCard(item));
    renderPages(byId("target-pages"), "target_cursor", page.next_cursor);
  } catch (error) {
    message(area, error.message, true);
    byId("target-pages").hidden = true;
  }
}
function showTarget(record) {
  const target = record.target;
  const panel = byId("selected-panel");
  const area = byId("selected-target");
  clear(area);
  add(area, node("strong", "", targetTitle(target)), metadata([
    ["Identity", target.identity], ["Board", target.board], ["Variant", target.variant],
    ["iOS version", target.version], ["Target ID", target.id], ["Artifact SHA-256", record.artifact.sha256],
    ["Source cache", record.artifact.source_availability === "registered_unverified" ? "Location recorded; bytes unverified" : "No location recorded"],
  ], "target-meta"));
  panel.hidden = false;
}
function runCard(run) {
  const card = node("article", `run-card${run.id === selectedRunID ? " selected" : ""}`);
  const link = anchor("", { run: run.id, finding: "", observation: "", evidence: "" }, [], "card-link");
  const top = node("div", "card-top");
  add(top, node("span", "card-title", shortID(run.id)), pill(run.state));
  add(link, top, node("div", "card-subtitle", `Started ${dateLabel(run.started)} · ${run.planned_components.length} planned components`));
  card.append(link);
  return card;
}
async function loadRuns() {
  const area = byId("run-results");
  message(area, "Loading runs…");
  try {
    const page = await api(queryURL("/api/v1/runs", {
      target_id: selectedTargetID, state: route.get("run_state"), cursor: route.get("run_cursor"), limit: 25,
    }));
    clear(area);
    if (!page.items.length) message(area, "No runs match this target and state. No inspection coverage can be inferred from an empty list.");
    else for (const item of page.items) area.append(runCard(item));
    renderPages(byId("run-pages"), "run_cursor", page.next_cursor);
  } catch (error) {
    message(area, error.message, true);
    byId("run-pages").hidden = true;
  }
}
function coverageCard(coverage) {
  const card = node("div", "coverage-item");
  const head = node("div", "card-top");
  add(head, node("strong", "", coverage.component), pill(coverage.state));
  card.append(head);
  if (coverage.reason) card.append(node("p", "", coverage.reason));
  const button = anchor("Browse component", { component: coverage.component, view: "observations", observation: "", finding: "", evidence: "", result_cursor: "" }, [], "button");
  card.append(button);
  return card;
}
async function loadRunDetail() {
  const area = byId("run-detail");
  if (!selectedRunID) { clear(area); return; }
  message(area, "Loading selected run…");
  try {
    const run = await api(`/api/v1/runs/${encodeURIComponent(selectedRunID)}`);
    if (run.target_id !== selectedTargetID) throw new Error("This run belongs to another target.");
    clear(area);
    const heading = node("div", "subsection");
    add(heading, node("h3", "", "Run scope"), metadata([
      ["Run ID", run.id], ["State", run.state], ["Started", dateLabel(run.started)],
      ["Finished", dateLabel(run.finished)], ["Input SHA-256", run.input_sha256],
      ["Input size", `${run.input_size} bytes`], ["Extractor", run.extractor],
    ]));
    area.append(heading);
    const checkpoints = node("div", "subsection");
    checkpoints.append(node("h3", "", "Committed checkpoints"));
    if (!run.committed_checkpoints || !run.committed_checkpoints.length) {
      checkpoints.append(node("p", "hint", "No component checkpoint has been committed."));
    } else {
      const grid = node("div", "coverage-grid");
      for (const item of run.committed_checkpoints) grid.append(coverageCard(item));
      checkpoints.append(grid);
    }
    area.append(checkpoints);
    const final = node("div", "subsection");
    final.append(node("h3", "", "Final coverage"));
    if (!run.final_coverage_available) {
      final.append(node("p", "message", "No final coverage ledger is available for this run. Checkpoints above are durable work, not a completed search."));
    } else {
      const grid = node("div", "coverage-grid");
      for (const item of run.final_coverage || []) grid.append(coverageCard(item));
      final.append(grid);
    }
    area.append(final);
    if (run.committed_batches && run.committed_batches.length) {
      const batches = node("p", "hint", `Committed batches: ${run.committed_batches.map((item) => `${item.component} ${item.count}`).join(" · ")}`);
      area.append(batches);
    }
    if (!route.has("finding") && !route.has("observation")) {
      area.scrollIntoView({ block: "start" });
    }
  } catch (error) { message(area, error.message, true); }
}
function componentName(observation, target) {
  const entries = Object.entries(target.components || {});
  const found = entries.find(([, member]) => member === observation.source?.member);
  return found ? found[0] : "Unmapped component";
}
function valueSummary(value) {
  const formatted = formatExactJSON(value);
  return formatted.length > 180 ? `${formatted.slice(0, 180)}…` : formatted;
}
function findingCard(finding) {
  const card = node("article", "finding-card");
  const link = anchor("", { view: "findings", finding: finding.id, observation: "", evidence: "" }, [], "card-link");
  add(link, node("div", "card-title", `${finding.subject} · ${finding.property}`),
    node("div", "card-subtitle", `${finding.revision_count} retained revision${String(finding.revision_count) === "1" ? "" : "s"} · ${shortID(finding.id)}`));
  card.append(link);
  return card;
}
function observationCard(observation, target) {
  const card = node("article", "observation-card");
  const link = anchor("", { view: "observations", observation: observation.id, finding: "", evidence: "" }, [], "card-link");
  const top = node("div", "card-top");
  add(top, node("span", "card-title", `${observation.subject} · ${observation.property}`), pill(observation.presence));
  add(link, top, node("div", "card-subtitle", `${componentName(observation, target)} · ${observation.presence === "present" ? valueSummary(observation.value) : safeLabel(observation.presence)} · ${shortID(observation.id)}`));
  card.append(link);
  return card;
}
function setComponents(target) {
  const select = byId("component");
  clear(select);
  const all = node("option", "", "All components");
  all.value = "";
  select.append(all);
  for (const component of Object.keys(target.components || {}).sort()) {
    const option = node("option", "", component);
    option.value = component;
    select.append(option);
  }
  select.value = route.get("component") || "";
  if (route.get("component") && !select.value) {
    const option = node("option", "", route.get("component"));
    option.value = route.get("component");
    select.append(option);
    select.value = option.value;
  }
}
function setupTabs() {
  for (const [id, selected] of [["findings-tab", "findings"], ["observations-tab", "observations"]]) {
    const tab = byId(id);
    tab.href = withRoute({ view: selected, finding: "", observation: "", evidence: "", result_cursor: "" });
    if (view === selected) tab.setAttribute("aria-current", "page");
    else tab.removeAttribute("aria-current");
  }
  for (const item of document.querySelectorAll(".finding-filter")) item.hidden = view !== "findings";
  byId("result-scope").textContent = view === "findings"
    ? "Findings cover this exact target across retained runs. A selected run filters observations only."
    : selectedRunID ? "Observations are filtered to the selected run and target." : "Observations cover all retained runs for this exact target.";
}
async function loadResults(target) {
  const area = byId("result-list");
  message(area, `Loading ${view}…`);
  try {
    const common = {
      target_id: selectedTargetID, subject: route.get("subject"), property: route.get("property"),
      component: route.get("component"), cursor: route.get("result_cursor"), limit: 25,
    };
    const filters = view === "findings"
      ? { ...common, basis: route.get("basis"), assessment: route.get("assessment") }
      : { ...common, run_id: selectedRunID };
    const page = await api(queryURL(`/api/v1/${view}`, filters));
    clear(area);
    if (!page.items.length) {
      message(area, `No retained ${view} match these filters. Check run coverage: unexamined or failed areas do not establish absence.`);
    } else {
      for (const item of page.items) area.append(view === "findings" ? findingCard(item) : observationCard(item, target));
    }
    renderPages(byId("result-pages"), "result_cursor", page.next_cursor);
    if (!route.has("finding") && !route.has("observation") && route.has("view") && !selectedRunID) {
      byId("browse-title").focus({ preventScroll: true });
      byId("browse-title").scrollIntoView({ block: "start" });
    }
  } catch (error) {
    message(area, error.message, true);
    byId("result-pages").hidden = true;
  }
}
function renderValue(parent, value, presence) {
  if (presence !== "present") {
    parent.append(node("p", "hint", presence === "absent" ? "Absent: no value was recorded." : "Value was not examined."));
    return;
  }
  const formatted = formatExactJSON(value);
  const limit = 65536;
  parent.append(node("pre", "value mono", formatted.length > limit ? `${formatted.slice(0, limit)}\n… display truncated after 65,536 characters` : formatted));
}
function sourceChain(source) {
  const wrapper = node("div", "source-chain");
  const steps = [
    ["Artifact", shortID(source.artifact_id)], ["Member", source.member],
    ["Coordinate", source.coordinate], ["Selector", source.selector],
  ];
  if ((source.offset && source.offset !== "0") || (source.length && source.length !== "0")) {
    steps.push(["Range", `${source.offset} + ${source.length} bytes`]);
  }
  for (const transform of source.transforms || []) steps.push(["Transform", transform]);
  for (const [index, [label, value]] of steps.entries()) {
    if (index) wrapper.append(node("span", "source-arrow", "→"));
    wrapper.append(node("span", "source-step", `${label}: ${safeLabel(value)}`));
  }
  return wrapper;
}
async function loadEvidence(parent, observation) {
  message(parent, "Checking retained evidence…");
  try {
    const preview = await api(queryURL(`/api/v1/evidence/${encodeURIComponent(observation.evidence_id)}/preview`, {
      target_id: selectedTargetID, bytes: 512,
    }));
    clear(parent);
    const state = node("div", "pill-row");
    add(state, pill(preview.state), pill(preview.digest_verified ? "complete" : "unexamined", preview.digest_verified ? "Digest verified" : "Digest unavailable"));
    parent.append(state);
    parent.append(node("p", "hint", `${preview.preview_bytes} of ${preview.declared_bytes} retained bytes shown${preview.truncated ? " · preview truncated" : ""}.`));
    if (preview.state !== "complete") {
      parent.append(node("p", "message", "Retained bytes are unavailable. The observation and source reference remain visible."));
      return;
    }
    if (preview.text !== undefined && preview.text !== "") {
      parent.append(node("h3", "", "Text preview"));
      parent.append(node("pre", "value mono", preview.text));
    } else {
      parent.append(node("h3", "", "Hex preview"));
      parent.append(node("pre", "value mono", preview.hex || "(zero bytes)"));
    }
  } catch (error) { message(parent, error.message, true); }
}
function observationBlock(observation, target, label = "Source observation") {
  const section = node("div", "subsection");
  section.append(node("h3", "", label));
  const chips = node("div", "pill-row");
  add(chips, pill(observation.presence), pill("muted", componentName(observation, target)));
  section.append(chips);
  section.append(metadata([
    ["Subject", observation.subject], ["Property", observation.property],
    ["Observation ID", observation.id], ["Evidence ID", observation.evidence_id],
    ["Extractor", observation.extractor],
  ]));
  renderValue(section, observation.value, observation.presence);
  section.append(node("h3", "", "Logical source"), sourceChain(observation.source || {}));
  const link = anchor("Open observation", { view: "observations", observation: observation.id, finding: "", evidence: "" }, [], "button");
  section.append(link);
  if (observation.evidence_id) {
    const previewLink = anchor("Preview retained bytes", { evidence: observation.evidence_id }, [], "button preview-link");
    section.append(previewLink);
    const preview = node("div", "preview");
    section.append(preview);
    if (route.get("evidence") === observation.evidence_id) loadEvidence(preview, observation);
  }
  return section;
}
function renderAssessmentHistory(parent, revision) {
  if (!revision.assessment_history || !revision.assessment_history.length) return;
  const history = node("div", "subsection");
  history.append(node("h3", "", "Assessment history"));
  for (const event of revision.assessment_history) {
    const row = node("div", "history-item");
    add(row, node("strong", "", `${event.state} · ${event.reviewer}`),
      node("div", "", `${dateLabel(event.created)} · ${event.reason}`));
    history.append(row);
  }
  parent.append(history);
}
function renderFinding(chain, target) {
  const area = byId("result-detail");
  clear(area);
  byId("detail-title").textContent = `${chain.subject} · ${chain.property}`;
  area.append(metadata([["Finding ID", chain.id], ["Target ID", chain.target_id]]));
  if (!chain.revisions || !chain.revisions.length) {
    area.append(node("p", "message", "This finding has no retained revisions."));
    return;
  }
  for (const revision of chain.revisions) {
    const card = node("article", "revision");
    card.id = `revision-${revision.id}`;
    const heading = node("div", "card-top");
    add(heading, node("strong", "", `Revision ${shortID(revision.id)}`), node("time", "hint", dateLabel(revision.created)));
    card.append(heading);
    const chips = node("div", "pill-row");
    add(chips, pill(revision.basis, `Basis: ${revision.basis}`),
      pill(revision.assessment, `Assessment: ${revision.assessment}`),
      pill(revision.presence, `Presence: ${revision.presence}`));
    card.append(chips);
    card.append(metadata([
      ["Author", revision.author], ["Rationale", revision.rationale],
      ["Supersedes", revision.supersedes || "None"], ["Revision ID", revision.id],
    ]));
    renderValue(card, revision.value, revision.presence);
    if (revision.observation) card.append(observationBlock(revision.observation, target));
    for (const reference of revision.references || []) {
      if (revision.observation && reference.observation_id === revision.observation.id) continue;
      if (reference.observation) card.append(observationBlock(reference.observation, target, `${reference.role} observation`));
      else card.append(node("p", "hint", `${reference.role} revision: ${reference.revision_id}`));
    }
    renderAssessmentHistory(card, revision);
    area.append(card);
  }
}
async function loadDetail(target) {
  const panel = byId("detail-panel");
  const area = byId("result-detail");
  const findingID = route.get("finding");
  const observationID = route.get("observation");
  if (!findingID && !observationID) { panel.hidden = true; return; }
  panel.hidden = false;
  message(area, "Loading result detail…");
  try {
    const kind = observationID ? "observations" : "findings";
    const id = observationID || findingID;
    const data = await api(queryURL(`/api/v1/${kind}/${encodeURIComponent(id)}`, { target_id: selectedTargetID }));
    if (data.target_id !== selectedTargetID) throw new Error("This result belongs to another target.");
    if (observationID) {
      clear(area);
      byId("detail-title").textContent = `${data.subject} · ${data.property}`;
      area.append(observationBlock(data, target, "Observation and source"));
    } else renderFinding(data, target);
    byId("detail-title").focus({ preventScroll: true });
    panel.scrollIntoView({ block: "start" });
  } catch (error) { message(area, error.message, true); }
}
function wireForms() {
  byId("target-form").addEventListener("submit", (event) => navigateForm(event, {
    product: byId("product").value.trim(), build: byId("build").value.trim(),
    identity: byId("identity").value, target: "", run: "", finding: "", observation: "", evidence: "",
    target_cursor: "", run_cursor: "", result_cursor: "",
  }));
  byId("run-form").addEventListener("submit", (event) => navigateForm(event, {
    run_state: byId("run-state").value, run_cursor: "", run: "", finding: "", observation: "", evidence: "",
  }));
  byId("result-form").addEventListener("submit", (event) => navigateForm(event, {
    subject: byId("subject").value.trim(), property: byId("property").value.trim(),
    component: byId("component").value, basis: byId("basis").value,
    assessment: byId("assessment").value, view, result_cursor: "", finding: "", observation: "", evidence: "",
  }));
}

function monitorURL(changes = {}) {
  const next = new URLSearchParams();
  next.set("screen", "monitor");
  const values = {
    monitor_kind: route.get("monitor_kind"), active_cursor: route.get("active_cursor"),
    history_cursor: route.get("history_cursor"), operation: route.get("operation"),
    operation_kind: route.get("operation_kind"), ...changes,
  };
  for (const [key, value] of Object.entries(values)) {
    if (value !== "" && value !== null && value !== undefined) next.set(key, String(value));
  }
  return `/?${next.toString()}`;
}

function monitorCard(item) {
  const progress = item.progress;
  const card = node("article", "monitor-card");
  const link = node("a", "card-link");
  link.href = monitorURL({ operation: progress.id, operation_kind: progress.kind });
  const title = progress.kind === "acquisition"
    ? `${safeLabel(item.product, "Firmware")} · ${safeLabel(item.version, "version unknown")} · ${safeLabel(item.build, "build unknown")}`
    : `Inspection · ${shortID(progress.id)}`;
  const top = node("div", "card-top");
  add(top, node("strong", "card-title", title), pill(item.effective_state));
  add(link, top, node("div", "card-subtitle", `${safeLabel(progress.phase)}${progress.component ? ` · ${progress.component}` : ""} · Updated ${dateLabel(progress.updated)}`));
  card.append(link);
  if (progress.kind === "acquisition") card.append(node("p", "hint", `${progress.completed_bytes} transferred · ${progress.verified_bytes} verified bytes`));
  else card.append(node("p", "hint", `${progress.completed_units} components · ${progress.published_batches} retained batches`));
  return card;
}

function monitorPager(id, key, cursor) {
  const area = byId(id);
  clear(area);
  const current = route.get(key) || "";
  area.hidden = !current && !cursor;
  if (current) {
    const first = node("a", "button", "First page");
    first.href = monitorURL({ [key]: "" });
    area.append(first);
  }
  if (cursor) {
    const next = node("a", "button", "Next page");
    next.href = monitorURL({ [key]: cursor });
    area.append(next);
  }
}

const monitorListSignatures = new Map();
function renderMonitorPage(areaID, page, empty) {
  const area = byId(areaID);
  const signature = page.items.map((item) => `${item.progress.id}:${item.progress.version}:${item.effective_state}`).join("|");
  if (monitorListSignatures.get(areaID) === signature) return;
  monitorListSignatures.set(areaID, signature);
  const focusedHref = area.contains(document.activeElement) ? document.activeElement.href : "";
  clear(area);
  if (!page.items.length) message(area, empty);
  else for (const item of page.items) area.append(monitorCard(item));
  if (focusedHref) {
    const sameLink = Array.from(area.querySelectorAll("a")).find((link) => link.href === focusedHref);
    if (sameLink) sameLink.focus({ preventScroll: true });
  }
}

async function loadMonitorLists(initial = false) {
  const kind = route.get("monitor_kind") || "";
  if (initial) {
    message(byId("monitor-active"), "Loading active operations…");
    message(byId("monitor-history"), "Loading recent history…");
  }
  try {
    const [active, history] = await Promise.all([
      api(queryURL("/api/v1/progress", { kind, state: "running", cursor: route.get("active_cursor"), limit: 25 })),
      api(queryURL("/api/v1/progress", { kind, state: "terminal", cursor: route.get("history_cursor"), limit: 25 })),
    ]);
    for (const [areaID, page, empty] of [
      ["monitor-active", active, "No active operations are retained."],
      ["monitor-history", history, "No finished operations are retained."],
    ]) {
      renderMonitorPage(areaID, page, empty);
    }
    monitorPager("monitor-active-pages", "active_cursor", active.next_cursor);
    monitorPager("monitor-pages", "history_cursor", history.next_cursor);
    byId("monitor-connection").textContent = "Showing committed database state";
  } catch (error) {
    if (initial) {
      message(byId("monitor-active"), error.message, true);
      message(byId("monitor-history"), error.message, true);
    }
    byId("monitor-connection").textContent = "Connection lost; last committed view retained";
  }
}

function monitorMeasure(parent, label, completed, total, state) {
  const section = node("div", "measure");
  const known = total !== undefined && total !== null;
  const text = known ? `${completed} of ${total}` : `${completed}; total unknown`;
  section.append(node("strong", "", `${label}: ${text}`));
  if (known) {
    const bar = node("progress");
    bar.max = Number(total) || 1;
    bar.value = Number(completed);
    bar.setAttribute("aria-label", `${label}: ${text}`);
    section.append(bar);
  } else if (state === "running") {
    const bar = node("progress");
    bar.setAttribute("aria-label", `${label}: ${text}`);
    section.append(bar);
  }
  parent.append(section);
}

function monitorCoverage(parent, run, targetID) {
  if (!run) return;
  const final = run.final_coverage_available;
  const coverage = final ? run.final_coverage : run.committed_checkpoints;
  const section = node("section", "subsection");
  section.append(node("h3", "", final ? "Final component coverage" : "Committed checkpoints so far"));
  if (!coverage || !coverage.length) {
    section.append(node("p", "message", "No component coverage has been retained yet."));
  } else {
    const grid = node("div", "coverage-grid");
    for (const item of coverage) {
      const card = node("div", "coverage-item");
      const head = node("div", "card-top");
      add(head, node("strong", "", item.component), pill(item.state));
      card.append(head);
      if (item.reason) card.append(node("p", "", item.reason));
      if (targetID) {
        const link = node("a", "button coverage-link", "Browse evidence");
        link.href = `/?${new URLSearchParams({ target: targetID, run: run.id, view: "observations", component: item.component })}`;
        card.append(link);
      }
      grid.append(card);
    }
    section.append(grid);
  }
  parent.append(section);
}

let monitorVersion = "";
let monitorCursor = "0";
const monitorEventSequences = new Set();
function renderMonitorSnapshot(snapshot) {
  const p = snapshot.progress;
  const fingerprint = `${p.version}:${snapshot.effective_state}:${snapshot.run && snapshot.run.final_coverage_available}`;
  if (monitorVersion === fingerprint) return;
  monitorVersion = fingerprint;
  const area = byId("monitor-detail");
  clear(area);
  const head = node("div", "card-top");
  add(head, node("strong", "", `${p.kind === "acquisition" ? "Acquisition attempt" : "Inspection run"} ${shortID(p.id)}`), pill(snapshot.effective_state));
  area.append(head);
  if (snapshot.stale) area.append(node("p", "notice notice-warning", "The inspection lease expired. This is the last committed progress; recovery has not marked the run terminal."));
  if (p.state === "interrupted") area.append(node("p", "notice notice-warning", "The worker stopped before this operation completed."));
  if (p.state === "partial") area.append(node("p", "notice notice-warning", "This operation completed with partial coverage. Review each component below."));
  const facts = [
    ["Operation ID", p.id], ["Phase", p.phase], ["State", snapshot.effective_state],
    ["Current component", p.component || "None"], ["Started", dateLabel(p.started)],
    ["Last committed", dateLabel(p.updated)], ["Finished", dateLabel(p.finished)],
    ["Diagnostic code", p.diagnostic_code || "None"],
  ];
  if (p.input_sha256) facts.push(["Input SHA-256", p.input_sha256]);
  area.append(metadata(facts));
  const measurements = node("div", "measure-grid");
  if (p.kind === "acquisition") {
    monitorMeasure(measurements, "Transferred bytes", p.completed_bytes, p.total_bytes, snapshot.effective_state);
    monitorMeasure(measurements, "Verified bytes", p.verified_bytes, p.total_bytes, snapshot.effective_state);
  } else {
    monitorMeasure(measurements, "Completed components", p.completed_units, p.total_units, snapshot.effective_state);
    measurements.append(node("p", "hint", `${p.published_batches} retained batches`));
  }
  area.append(measurements);
  if (snapshot.run) {
    const run = snapshot.run;
    const link = node("a", "button", "Browse this exact run");
    link.href = `/?${new URLSearchParams({ target: run.target_id, run: run.id })}`;
    area.append(link);
    monitorCoverage(area, run, run.target_id);
  }
}

function renderMonitorEvents(page) {
  const area = byId("monitor-events");
  if (page.gap) {
    byId("monitor-gap").hidden = false;
    byId("monitor-gap").textContent = `Earlier phase events expired from retention. Oldest retained sequence: ${page.oldest_sequence}.`;
    clear(area);
    monitorEventSequences.clear();
  }
  for (const event of page.events) {
    if (area.querySelector(":scope > .message")) clear(area);
    const sequence = String(event.sequence);
    if (monitorEventSequences.has(sequence)) continue;
    monitorEventSequences.add(sequence);
    const row = node("div", "history-item");
    add(row, node("strong", "", `${event.phase} · ${event.state}`),
      node("div", "", `${dateLabel(event.created)}${event.component ? ` · ${event.component}` : ""}${event.diagnostic_code ? ` · ${event.diagnostic_code}` : ""}`));
    area.append(row);
  }
  if (page.events.length) monitorCursor = page.next_cursor;
  if (!area.children.length) message(area, "No retained phase events yet.");
}

let monitorPolling = false;
async function pollMonitorDetail(initial = false) {
  const operation = route.get("operation");
  const kind = route.get("operation_kind");
  if (!operation || !["acquisition", "inspection"].includes(kind)) return;
  if (monitorPolling) return;
  monitorPolling = true;
  try {
    const resource = kind === "acquisition" ? "acquisitions" : "runs";
    const page = await api(queryURL(`/api/v1/${resource}/${encodeURIComponent(operation)}/progress/events`, { since: monitorCursor, limit: 64 }));
    if (page.snapshot.progress.id !== operation || page.snapshot.progress.kind !== kind) throw new Error("Operation identity changed.");
    renderMonitorSnapshot(page.snapshot);
    renderMonitorEvents(page);
    byId("monitor-detail-status").textContent = `Connected · last committed ${dateLabel(page.snapshot.progress.updated)}`;
    if (initial) byId("monitor-detail-title").focus({ preventScroll: true });
    if (page.has_more) setTimeout(() => pollMonitorDetail(), 0);
  } catch (error) {
    byId("monitor-detail-status").textContent = `Connection lost; last committed detail retained. ${error.message}`;
  } finally { monitorPolling = false; }
}

function startMonitor() {
  document.title = "Monitor work · go-ios-vm";
  byId("page-title").textContent = "Monitor local work";
  byId("page-lede").textContent = "Watch committed acquisition and inspection progress, then revisit its phase history and coverage.";
  byId("browse-layout").hidden = true;
  byId("monitor-workspace").hidden = false;
  byId("monitor-nav").setAttribute("aria-current", "page");
  byId("monitor-kind").value = route.get("monitor_kind") || "";
  byId("monitor-filter").addEventListener("submit", (event) => {
    event.preventDefault();
    window.location.assign(monitorURL({ monitor_kind: byId("monitor-kind").value, active_cursor: "", history_cursor: "" }));
  });
  loadMonitorLists(true);
  setInterval(() => loadMonitorLists(), 5000);
  if (route.get("operation")) {
    byId("monitor-detail-panel").hidden = false;
    pollMonitorDetail(true);
    setInterval(() => pollMonitorDetail(), 2000);
  }
}

async function start() {
  if (screen === "monitor") {
    checkHealth();
    setInterval(checkHealth, 10000);
    startMonitor();
    return;
  }
  byId("browse-nav").setAttribute("aria-current", "page");
  for (const [id, key] of [["product", "product"], ["build", "build"], ["identity", "identity"],
    ["run-state", "run_state"], ["subject", "subject"], ["property", "property"],
    ["basis", "basis"], ["assessment", "assessment"]]) setFormValue(id, key);
  wireForms();
  checkHealth();
  setInterval(checkHealth, 10000);
  loadTargets();
  if (!selectedTargetID) return;
  byId("welcome").hidden = true;
  byId("workspace").hidden = false;
  setupTabs();
  try {
    const targetRecord = await api(`/api/v1/targets/${encodeURIComponent(selectedTargetID)}`);
    showTarget(targetRecord);
    setComponents(targetRecord.target);
    await Promise.all([loadRuns(), loadRunDetail(), loadResults(targetRecord.target), loadDetail(targetRecord.target)]);
  } catch (error) {
    byId("global-error").hidden = false;
    byId("global-error").textContent = error.message;
    byId("workspace").hidden = true;
  }
}

start();
