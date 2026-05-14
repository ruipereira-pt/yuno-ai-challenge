"use client";

import { useEffect, useMemo, useState } from "react";

const POLL_MS = 2000;

function fmtPct(v) {
  if (typeof v !== "number") return "-";
  return `${(v * 100).toFixed(1)}%`;
}

function fmtNum(v, digits = 1) {
  if (typeof v !== "number") return "-";
  return v.toFixed(digits);
}

export default function DashboardPage() {
  const [health, setHealth] = useState(null);
  const [alerts, setAlerts] = useState(null);
  const [comparison, setComparison] = useState(null);
  const [lastUpdated, setLastUpdated] = useState(null);
  const [error, setError] = useState("");

  useEffect(() => {
    let timer = null;
    let mounted = true;

    async function tick() {
      try {
        const [hRes, aRes, cRes] = await Promise.all([
          fetch("/api/health", { cache: "no-store" }),
          fetch("/api/alerts?active_only=true", { cache: "no-store" }),
          fetch("/api/comparison", { cache: "no-store" })
        ]);

        if (!hRes.ok || !aRes.ok || !cRes.ok) {
          throw new Error(`API error health=${hRes.status} alerts=${aRes.status} comparison=${cRes.status}`);
        }

        const [h, a, c] = await Promise.all([hRes.json(), aRes.json(), cRes.json()]);
        if (!mounted) return;
        setHealth(h);
        setAlerts(a);
        setComparison(c);
        setLastUpdated(new Date());
        setError("");
      } catch (e) {
        if (!mounted) return;
        setError(e.message || "Failed to fetch dashboard data");
      }
    }

    tick();
    timer = setInterval(tick, POLL_MS);
    return () => {
      mounted = false;
      if (timer) clearInterval(timer);
    };
  }, []);

  const alertByPSP = useMemo(() => {
    const map = new Map();
    const events = alerts?.events || [];
    for (const e of events) {
      if (!map.has(e.psp)) map.set(e.psp, e);
    }
    return map;
  }, [alerts]);

  return (
    <main className="page">
      <div className="header">
        <h1 className="title">PSP Health Dashboard</h1>
        <div className="meta">
          Polling every {POLL_MS / 1000}s
          {lastUpdated ? ` • updated ${lastUpdated.toLocaleTimeString()}` : ""}
        </div>
      </div>

      {error ? <p className="muted">Error: {error}</p> : null}

      <section className="grid">
        {(health?.psps || []).map((psp) => {
          const active = alertByPSP.get(psp.psp);
          const five = psp.windows?.["5m"];
          return (
            <article key={psp.psp} className="card">
              <h3>{psp.psp}</h3>
              <div>
                <span className={`badge ${psp.degraded ? "bad" : "ok"}`}>
                  {psp.degraded ? "DEGRADED" : "HEALTHY"}
                </span>
              </div>
              <p>Health score: {psp.health_score ?? "-"}</p>
              <p>5m approval: {fmtPct(five?.approval_rate)}</p>
              <p>5m error: {fmtPct(five?.error_rate)}</p>
              <p>5m latency: {fmtNum(five?.avg_response_time_ms)} ms</p>
              <p className="muted">Active alert: {active ? `${active.reason} since ${active.started_at}` : "none"}</p>
            </article>
          );
        })}
      </section>

      <section className="section">
        <h2>Active Alerts</h2>
        <table className="table">
          <thead>
            <tr>
              <th>PSP</th>
              <th>Started At</th>
              <th>Reason</th>
            </tr>
          </thead>
          <tbody>
            {(alerts?.events || []).length === 0 ? (
              <tr>
                <td colSpan={3} className="muted">
                  No active alerts
                </td>
              </tr>
            ) : (
              (alerts?.events || []).map((e, idx) => (
                <tr key={`${e.psp}-${e.started_at}-${idx}`}>
                  <td>{e.psp}</td>
                  <td>{e.started_at}</td>
                  <td>{e.reason}</td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </section>

      <section className="section">
        <h2>Current Ranking</h2>
        <table className="table">
          <thead>
            <tr>
              <th>Rank</th>
              <th>PSP</th>
              <th>Health Score</th>
              <th>Approval Rate</th>
              <th>Avg Latency (ms)</th>
            </tr>
          </thead>
          <tbody>
            {(comparison?.ranking || []).map((r, idx) => (
              <tr key={r.psp}>
                <td>{idx + 1}</td>
                <td>{r.psp}</td>
                <td>{fmtNum(r.health_score)}</td>
                <td>{fmtPct(r.approval_rate)}</td>
                <td>{fmtNum(r.avg_response_time_ms)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>
    </main>
  );
}
