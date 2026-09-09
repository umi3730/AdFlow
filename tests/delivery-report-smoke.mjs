import assert from "node:assert/strict";

// Run only against an explicitly isolated, disposable local API.
const base = process.env.ADFLOW_REPORT_SMOKE_URL ?? "http://127.0.0.1:18082";
const target = new URL(base);
if (!["localhost", "127.0.0.1"].includes(target.hostname) || target.port !== "18082")
  throw new Error("Use the isolated report API on port 18082");
async function request(path, body, method = body ? "POST" : "GET") {
  const r = await fetch(base + path, {
    method,
    headers: { "Content-Type": "application/json" },
    ...(body ? { body: JSON.stringify(body) } : {}),
  });
  const text = await r.text();
  const result = text ? JSON.parse(text) : undefined;
  assert.ok(r.ok, JSON.stringify(result));
  return result;
}
const stamp = Date.now();
const now = new Date();
const slotId = `report-${stamp}`;
const campaign = await request("/v1/campaigns", {
  name: `报表联调 ${stamp}`,
  slotId,
  startAt: new Date(now.getTime() - 3600000).toISOString(),
  endAt: new Date(now.getTime() + 86400000).toISOString(),
});
await request(`/v1/campaigns/${campaign.id}/creatives`, {
  title: "报表验证素材",
  description: "本地联调",
  imageUrl: "http://127.0.0.1:3001/favicon.ico",
  landingUrl: "http://127.0.0.1:3001/",
});
await request(`/v1/campaigns/${campaign.id}/publish`, {
  targeting: { all: [{ tag: "anime" }], any: [], none: [] },
  dailyBudgetFen: 100000,
  impressionCostFen: 7,
  frequencyLimit: 3,
});
let lastRequest;
for (let i = 0; i < 3; i++) {
  const user = `report-${stamp}-${i}`;
  await request(
    `/v1/profiles/${user}`,
    { tags: ["anime"], fields: { device: "android", score: "80" } },
    "PUT",
  );
  const requestId = `report-r-${stamp}-${i}`;
  lastRequest = requestId;
  const decision = await request("/v1/decisions", { requestId, userId: user, slotId });
  assert.equal(decision.matched, true);
  for (const type of [
    "impression",
    ...(i < 2 ? ["click"] : []),
    ...(i === 0 ? ["conversion"] : []),
  ]) {
    const event = {
      eventId: `${requestId}-${type}`,
      requestId,
      campaignId: decision.campaignId,
      creativeId: decision.creativeId,
      type,
      ...(type === "conversion" ? { valueFen: 300 } : {}),
    };
    await request("/v1/events", event);
    const duplicate = await request("/v1/events", event);
    assert.equal(duplicate.recorded, false);
  }
}
const filter = {
  from: new Date(now.getTime() - 3600000).toISOString(),
  to: new Date(now.getTime() + 3600000).toISOString(),
  granularity: "hour",
  campaignId: campaign.id,
};
const params = new URLSearchParams(filter);
const report = await request(`/v1/reports/delivery?${params}`);
assert.equal(report.summary.impressions, 3);
assert.equal(report.summary.clicks, 2);
assert.equal(report.summary.conversions, 1);
assert.equal(report.summary.spendFen, 21);
assert.equal(report.summary.valueFen, 300);
assert.equal(report.summary.ctr, 2 / 3);
assert.equal(report.summary.cvr, 0.5);
const daily = await request(
  `/v1/reports/delivery?${new URLSearchParams({ ...filter, granularity: "day" })}`,
);
assert.deepEqual(daily.summary, report.summary);
const csv = await fetch(`${base}/v1/reports/delivery/export?${params}`);
assert.equal(csv.status, 200);
assert.match(csv.headers.get("content-disposition"), /attachment/);
assert.match(await csv.text(), /CTR/);
const diagnosis = await request("/v1/agent/delivery-diagnoses", {
  ...filter,
  requestId: lastRequest,
  question: "分析当前投放效果",
});
assert.equal(diagnosis.provider, "mock");
assert.ok(diagnosis.evidence.some((e) => e.id === "request"));
assert.ok(diagnosis.evidence.some((e) => e.id === "rules"));
assert.ok(diagnosis.recommendations.length > 0);
const ids = new Set(diagnosis.evidence.map((e) => e.id));
for (const rec of diagnosis.recommendations) assert.ok(rec.evidenceIds.every((id) => ids.has(id)));
const missing = await fetch(base + "/v1/agent/delivery-diagnoses", {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({ ...filter, requestId: "not-found" }),
});
assert.equal(missing.status, 404);
console.log(
  JSON.stringify(
    {
      passed: true,
      campaignId: campaign.id,
      summary: report.summary,
      hourBuckets: report.series.length,
      dayBuckets: daily.series.length,
      diagnosisRecommendations: diagnosis.recommendations.length,
    },
    null,
    2,
  ),
);
