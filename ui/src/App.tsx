import { useEffect, useMemo, useState } from "react";

type LatestTelemetry = {
  device_id: string;
  ts: string;
  kw_usage: number;
  battery_soc_pct: number;
  heatpump_status: string;
  grid_price_eur_kwh: number;
  low_battery: boolean;
};

type AlertRow = {
  id: number;
  device_id: string;
  kind: string;
  payload: string | Record<string, unknown>;
  created_at: string;
};

type StrategyResponse = {
  device_id: string;
  samples: number;
  plan: {
    strategy: string;
    actions: Array<{ when: string; what: string; why: string }>;
  };
};

async function getJSON<T>(path: string): Promise<T> {
  const resp = await fetch(path);
  if (!resp.ok) {
    const body = await resp.text();
    throw new Error(`${resp.status} ${resp.statusText}: ${body}`);
  }
  return (await resp.json()) as T;
}

export function App() {
  const [deviceId, setDeviceId] = useState("home-00042");
  const [latest, setLatest] = useState<LatestTelemetry | null>(null);
  const [alerts, setAlerts] = useState<AlertRow[]>([]);
  const [strategy, setStrategy] = useState<StrategyResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string>("");
  const [lastRefresh, setLastRefresh] = useState<string>("");

  const batteryTone = useMemo(() => {
    if (!latest) return "neutral";
    if (latest.battery_soc_pct < 20) return "danger";
    if (latest.battery_soc_pct < 50) return "warning";
    return "ok";
  }, [latest]);

  async function refresh() {
    setLoading(true);
    setError("");
    try {
      const [latestRow, alertRows] = await Promise.all([
        getJSON<LatestTelemetry>(`/api/devices/${deviceId}/latest`),
        getJSON<AlertRow[]>("/api/alerts"),
      ]);
      setLatest(latestRow);
      setAlerts(alertRows);
      setLastRefresh(new Date().toLocaleTimeString());
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }

  async function fetchStrategy() {
    setLoading(true);
    setError("");
    try {
      const data = await getJSON<StrategyResponse>(`/api/devices/${deviceId}/strategy`);
      setStrategy(data);
      setLastRefresh(new Date().toLocaleTimeString());
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    refresh();
    const id = window.setInterval(refresh, 5000);
    return () => window.clearInterval(id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [deviceId]);

  return (
    <main className="layout">
      <header className="header">
        <div>
          <h1>VPP Operator Console</h1>
          <p>Live simulation view for telemetry, alerts, and AI strategy.</p>
        </div>
        <div className="actions">
          <input value={deviceId} onChange={(e) => setDeviceId(e.target.value)} />
          <button onClick={refresh} disabled={loading}>
            Refresh
          </button>
          <button onClick={fetchStrategy} disabled={loading}>
            Generate Strategy
          </button>
        </div>
      </header>

      {error ? <div className="error">{error}</div> : null}

      <section className="grid">
        <article className="card">
          <h2>Latest Telemetry</h2>
          {latest ? (
            <dl>
              <div>
                <dt>Device</dt>
                <dd>{latest.device_id}</dd>
              </div>
              <div>
                <dt>Timestamp</dt>
                <dd>{new Date(latest.ts).toLocaleString()}</dd>
              </div>
              <div>
                <dt>Usage</dt>
                <dd>{latest.kw_usage.toFixed(2)} kW</dd>
              </div>
              <div>
                <dt>Battery</dt>
                <dd className={`tone-${batteryTone}`}>{latest.battery_soc_pct.toFixed(1)}%</dd>
              </div>
              <div>
                <dt>Heatpump</dt>
                <dd>{latest.heatpump_status}</dd>
              </div>
              <div>
                <dt>Grid Price</dt>
                <dd>{latest.grid_price_eur_kwh.toFixed(3)} EUR/kWh</dd>
              </div>
            </dl>
          ) : (
            <p>No telemetry yet.</p>
          )}
        </article>

        <article className="card">
          <h2>Recent Alerts</h2>
          {alerts.length === 0 ? (
            <p>No alerts yet.</p>
          ) : (
            <ul className="alerts">
              {alerts.slice(0, 8).map((a) => (
                <li key={a.id}>
                  <strong>{a.kind}</strong> for <code>{a.device_id}</code>
                  <span>{new Date(a.created_at).toLocaleTimeString()}</span>
                </li>
              ))}
            </ul>
          )}
        </article>
      </section>

      <section className="card">
        <h2>AI Optimization Strategy</h2>
        {!strategy ? (
          <p>Click "Generate Strategy" to ask the optimizer.</p>
        ) : (
          <>
            <p>
              <strong>Samples:</strong> {strategy.samples}
            </p>
            <p>{strategy.plan.strategy}</p>
            <table>
              <thead>
                <tr>
                  <th>When</th>
                  <th>What</th>
                  <th>Why</th>
                </tr>
              </thead>
              <tbody>
                {strategy.plan.actions.map((a, idx) => (
                  <tr key={`${a.when}-${idx}`}>
                    <td>{a.when}</td>
                    <td>{a.what}</td>
                    <td>{a.why}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </>
        )}
      </section>

      <footer className="footer">
        <span>Data auto-refresh: every 5s</span>
        <span>Last refresh: {lastRefresh || "-"}</span>
      </footer>
    </main>
  );
}
