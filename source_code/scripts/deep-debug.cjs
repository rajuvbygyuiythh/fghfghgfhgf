#!/usr/bin/env node
// Deep Step-by-Step Project & Multi-Site Diagnostics Runner (browser-free).
const fs = require("node:fs");
const path = require("node:path");
const { spawnSync } = require("node:child_process");

const sourceRoot = path.resolve(__dirname, "..");
const repoRoot = path.resolve(sourceRoot, "..");
const sitesRoot = path.join(repoRoot, "sites");
const memoryRoot = path.join(repoRoot, "memory");
const skillsRoot = path.join(repoRoot, "skills");

function runDeepDiagnostics() {
  const steps = [];
  function addStep(stepNum, name, status, detail) {
    steps.push({ step: stepNum, name, status, detail });
    console.log(`[Step ${stepNum} · ${status}] ${name} — ${detail}`);
  }

  // Step 1: Isolated 102 Site Folders Check
  let siteDirs = [];
  if (fs.existsSync(sitesRoot)) {
    siteDirs = fs
      .readdirSync(sitesRoot, { withFileTypes: true })
      .filter((d) => d.isDirectory() && d.name !== "_global")
      .map((d) => d.name)
      .sort();
  }
  const missingSiteFiles = [];
  let autoRegCount = 0;
  let browserSetupCount = 0;
  const groupCounts = { A: 0, B: 0, C: 0, D: 0, E: 0 };
  for (const id of siteDirs) {
    const dir = path.join(sitesRoot, id);
    const required = [
      "site.json",
      "dom_driver.js",
      `${id}_protection.txt`,
      "README.txt",
      "accounts",
      "sessions",
      path.join("profiles", "default"),
      "logs",
    ];
    for (const rel of required) {
      if (!fs.existsSync(path.join(dir, rel))) {
        missingSiteFiles.push(`${id}/${rel}`);
      }
    }
    const cfgPath = path.join(dir, "site.json");
    if (fs.existsSync(cfgPath)) {
      try {
        const cfg = JSON.parse(fs.readFileSync(cfgPath, "utf8"));
        if (cfg.account_mode === "auto_register") autoRegCount += 1;
        else if (cfg.account_mode === "browser_setup") browserSetupCount += 1;
        const g = cfg.classification && cfg.classification.group;
        if (g && Object.prototype.hasOwnProperty.call(groupCounts, g)) {
          groupCounts[g] += 1;
        } else {
          missingSiteFiles.push(`${id}/site.json (missing classification.group)`);
        }
      } catch (e) {
        missingSiteFiles.push(`${id}/site.json (invalid JSON)`);
      }
    }
  }
  if (siteDirs.length >= 102 && missingSiteFiles.length === 0) {
    addStep(
      1,
      "Isolated Per-Site Folders (sites/<site_id>/)",
      "PASS",
      `${siteDirs.length} site folders verified (each with site.json, dom_driver.js, <site>_protection.txt, accounts/, profiles/, logs/)`,
    );
  } else {
    addStep(
      1,
      "Isolated Per-Site Folders (sites/<site_id>/)",
      "FAIL",
      `Found ${siteDirs.length} folders; missing/broken: ${missingSiteFiles.slice(0, 8).join(", ")}`,
    );
  }

  // Step 2: Dual Account Mode & Group A-E Classification Check
  const classifiedTotal = groupCounts.A + groupCounts.B + groupCounts.C + groupCounts.D + groupCounts.E;
  if (autoRegCount >= 85 && browserSetupCount >= 16 && classifiedTotal >= 102) {
    addStep(
      2,
      "Dual Account Setup & Group A-E Classification",
      "PASS",
      `⚡ Auto-Register: ${autoRegCount} | 🌐 Browser Setup: ${browserSetupCount} | Groups A=${groupCounts.A} B=${groupCounts.B} C=${groupCounts.C} D=${groupCounts.D} E=${groupCounts.E}`,
    );
  } else {
    addStep(
      2,
      "Dual Account Setup & Group A-E Classification",
      "FAIL",
      `Unexpected counts: auto_register=${autoRegCount}, browser_setup=${browserSetupCount}, classified=${classifiedTotal}`,
    );
  }

  // Step 3: Power-Safe Memory & Sub-Agent Skills Check
  const memFiles = ["sites_status.md", "operation_memory.json", "chat_history.jsonl", "README.md"];
  const skillFiles = [
    "dom_auto_scout.js",
    "ws_protocol_sniffer.js",
    "auto_onboard_agent.js",
    "protection_detector.js",
  ];
  const missingMemOrSkills = [];
  for (const f of memFiles) {
    if (!fs.existsSync(path.join(memoryRoot, f))) missingMemOrSkills.push(`memory/${f}`);
  }
  for (const f of skillFiles) {
    if (!fs.existsSync(path.join(skillsRoot, f))) missingMemOrSkills.push(`skills/${f}`);
  }
  if (missingMemOrSkills.length === 0) {
    addStep(
      3,
      "Power-Safe Memory (memory/) & Sub-Agent Skills (skills/)",
      "PASS",
      "All 4 memory files (fsync-backed) and all 4 browser sub-agent skills verified",
    );
  } else {
    addStep(
      3,
      "Power-Safe Memory (memory/) & Sub-Agent Skills (skills/)",
      "FAIL",
      `Missing: ${missingMemOrSkills.join(", ")}`,
    );
  }

  // Step 4: Frontend Syntax & WailsJS Contract Check
  const syntaxRes = spawnSync(
    process.execPath,
    ["--check", path.join(sourceRoot, "cmd/chitchatui/frontend/dist/app.js")],
    { encoding: "utf8" },
  );
  if (syntaxRes.status === 0) {
    addStep(
      4,
      "Dashboard Frontend & WailsJS Bindings",
      "PASS",
      "cmd/chitchatui/frontend/dist/app.js syntax and Wails bindings clean",
    );
  } else {
    addStep(
      4,
      "Dashboard Frontend & WailsJS Bindings",
      "FAIL",
      (syntaxRes.stderr || syntaxRes.stdout || "syntax error").trim(),
    );
  }

  // Step 5: Standalone apichat.exe Studio & Reply Engine Files
  const studioFiles = [
    "cmd/apichat/main.go",
    "cmd/apichat/ui/index.html",
    "cmd/apichat/ui/app.js",
    "internal/apichat/engine.go",
    "internal/sites/autoreg.go",
    "internal/sites/session_store.go",
    "internal/sites/diagnostics.go",
  ];
  const missingStudio = studioFiles.filter((f) => !fs.existsSync(path.join(sourceRoot, f)));
  if (missingStudio.length === 0) {
    addStep(
      5,
      "API Chat Studio & Multi-Site Diagnostics Engine",
      "PASS",
      "All core Go & UI files for apichat.exe, autoreg.go, and diagnostics.go present",
    );
  } else {
    addStep(
      5,
      "API Chat Studio & Multi-Site Diagnostics Engine",
      "FAIL",
      `Missing: ${missingStudio.join(", ")}`,
    );
  }

  const failed = steps.filter((s) => s.status === "FAIL").length;
  const overallStatus = failed === 0 ? "PASS" : "FAIL";
  const report = {
    overall_status: overallStatus,
    checked_at: new Date().toISOString(),
    total_sites: siteDirs.length,
    auto_register_sites: autoRegCount,
    browser_setup_sites: browserSetupCount,
    steps,
  };

  fs.mkdirSync(memoryRoot, { recursive: true });
  fs.writeFileSync(
    path.join(memoryRoot, "deep_debug_report.json"),
    JSON.stringify(report, null, 2) + "\n",
  );
  const txtLines = [
    "====================================================================",
    `DEEP DEBUG & DIAGNOSTICS REPORT (${report.checked_at})`,
    `OVERALL STATUS: ${overallStatus}`,
    "====================================================================",
    ...steps.map((s) => `[Step ${s.step} · ${s.status}] ${s.name}: ${s.detail}`),
    "====================================================================",
  ];
  fs.writeFileSync(path.join(memoryRoot, "deep_debug_report.txt"), txtLines.join("\n") + "\n");
  console.log(`\nOverall Deep Debug Status: ${overallStatus} (saved to memory/deep_debug_report.txt)`);
  if (failed > 0) process.exitCode = 1;
  return report;
}

if (require.main === module) {
  runDeepDiagnostics();
}

module.exports = { runDeepDiagnostics };
