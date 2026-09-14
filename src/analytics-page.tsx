import { createSignal, For, Show } from "solid-js";
import * as stylex from "@stylexjs/stylex";
import {
  AnalyticsAPIError,
  fetchAnalyticsDashboard,
  type AnalyticsBreakdown,
  type AnalyticsDashboard,
} from "./analytics-api";
import { analyticsStyles as styles } from "./analytics.stylex";

const sx = stylex.attrs;
const ranges = [7, 30, 90] as const;

export function AnalyticsPage() {
  const [dashboard, setDashboard] = createSignal<AnalyticsDashboard>();
  const [days, setDays] = createSignal(30);
  const [loading, setLoading] = createSignal(true);
  const [error, setError] = createSignal("");
  const [status, setStatus] = createSignal(0);

  void load(30);

  async function load(nextDays: number) {
    setDays(nextDays);
    setLoading(true);
    setError("");
    setStatus(0);
    try {
      setDashboard(await fetchAnalyticsDashboard(nextDays));
    } catch (reason) {
      if (reason instanceof AnalyticsAPIError) setStatus(reason.status);
      setError(reason instanceof Error ? reason.message : "analytics request failed");
    } finally {
      setLoading(false);
    }
  }

  const clickRate = () => {
    const data = dashboard();
    if (!data || data.summary.profile_views === 0) return "0%";
    return `${((data.summary.link_clicks / data.summary.profile_views) * 100).toFixed(1)}%`;
  };

  return (
    <section {...sx(styles.page)}>
      <div {...sx(styles.heading)}>
        <div>
          <div {...sx(styles.eyebrow)}>ANALYTICS</div>
          <h1 {...sx(styles.title)}>Know what resonates.</h1>
        </div>
        <div {...sx(styles.rangeGroup)} aria-label="Analytics date range">
          <For each={ranges}>
            {(range) => (
              <button
                {...sx(styles.rangeButton, days() === range && styles.activeRange)}
                type="button"
                disabled={loading()}
                aria-pressed={days() === range ? "true" : "false"}
                onClick={() => void load(range)}
              >
                {range} days
              </button>
            )}
          </For>
        </div>
      </div>

      <p {...sx(styles.intro)}>
        Profile views and link clicks are aggregated without storing raw IP addresses or full browsing histories.
        Visitor estimates deliberately rotate every UTC day.
      </p>

      <Show when={error()}>
        <div {...sx(styles.notice, styles.error)} role="alert">
          {error()}
          <Show when={status() === 401}> <a {...sx(styles.link)} href="/signin?next=%2Fanalytics">Sign in</a> to view your creator analytics.</Show>
          <Show when={status() === 409}> <a {...sx(styles.link)} href="/create">Claim your Vuta</a> first.</Show>
        </div>
      </Show>

      <Show when={!loading()} fallback={<div {...sx(styles.empty)}>Loading creator analytics…</div>}>
        <Show when={dashboard()}>
          {(data) => (
            <>
              <div {...sx(styles.summaryGrid)}>
                <Metric label="PROFILE VIEWS" value={formatNumber(data().summary.profile_views)} help={`${data().start_date} → ${data().end_date}`} />
                <Metric label="LINK CLICKS" value={formatNumber(data().summary.link_clicks)} help="Tracked through Vutame's safe redirect." />
                <Metric label="DAILY UNIQUES" value={formatNumber(data().summary.unique_visitors)} help="Sum of daily privacy-rotating visitor estimates." />
                <Metric label="CLICK / VIEW" value={clickRate()} help="Clicks divided by profile views; not a person-level conversion rate." />
              </div>

              <div {...sx(styles.grid)}>
                <div {...sx(styles.stack)}>
                  <section {...sx(styles.panel)}>
                    <div {...sx(styles.panelHeader)}>
                      <h2 {...sx(styles.panelTitle)}>Daily activity</h2>
                      <span {...sx(styles.subtle)}>{data().days} UTC days</span>
                    </div>
                    <div {...sx(styles.tableWrap)}>
                      <table {...sx(styles.table)}>
                        <thead>
                          <tr>
                            <th {...sx(styles.th)}>DATE</th>
                            <th {...sx(styles.th, styles.numeric)}>VIEWS</th>
                            <th {...sx(styles.th, styles.numeric)}>CLICKS</th>
                            <th {...sx(styles.th, styles.numeric)}>DAILY UNIQUES</th>
                          </tr>
                        </thead>
                        <tbody>
                          <For each={data().series.slice().reverse()}>
                            {(point) => (
                              <tr>
                                <td {...sx(styles.td)}>{point.date}</td>
                                <td {...sx(styles.td, styles.numeric)}>{formatNumber(point.profile_views)}</td>
                                <td {...sx(styles.td, styles.numeric)}>{formatNumber(point.link_clicks)}</td>
                                <td {...sx(styles.td, styles.numeric)}>{formatNumber(point.unique_visitors)}</td>
                              </tr>
                            )}
                          </For>
                        </tbody>
                      </table>
                    </div>
                  </section>

                  <section {...sx(styles.panel)}>
                    <div {...sx(styles.panelHeader)}>
                      <h2 {...sx(styles.panelTitle)}>Top links</h2>
                      <span {...sx(styles.subtle)}>By tracked clicks</span>
                    </div>
                    <Show when={data().top_links.length > 0} fallback={<div {...sx(styles.empty)}>No link clicks in this range yet.</div>}>
                      <div {...sx(styles.tableWrap)}>
                        <table {...sx(styles.table)}>
                          <thead><tr><th {...sx(styles.th)}>LINK</th><th {...sx(styles.th, styles.numeric)}>CLICKS</th></tr></thead>
                          <tbody>
                            <For each={data().top_links}>
                              {(link) => <tr><td {...sx(styles.td)}>{link.label}</td><td {...sx(styles.td, styles.numeric)}>{formatNumber(link.clicks)}</td></tr>}
                            </For>
                          </tbody>
                        </table>
                      </div>
                    </Show>
                  </section>
                </div>

                <div {...sx(styles.stack)}>
                  <BreakdownPanel title="Referrers" subtitle="Host only" items={data().referrers} />
                  <BreakdownPanel title="Devices" subtitle="Coarse class" items={data().devices} />
                </div>
              </div>

              <div {...sx(styles.privacy)}>
                <strong>Privacy model:</strong> Vutame does not store raw visitor IPs, raw user-agent strings, or full referrer URLs.
                Daily visitor tokens are creator-scoped and rotate each UTC day, so the same visitor cannot be joined across creators or reliably tracked across days from analytics storage.
              </div>
            </>
          )}
        </Show>
      </Show>
    </section>
  );
}

function Metric(props: { label: string; value: string; help: string }) {
  return (
    <div {...sx(styles.metric)}>
      <span {...sx(styles.metricLabel)}>{props.label}</span>
      <strong {...sx(styles.metricValue)}>{props.value}</strong>
      <div {...sx(styles.metricHelp)}>{props.help}</div>
    </div>
  );
}

function BreakdownPanel(props: { title: string; subtitle: string; items: AnalyticsBreakdown[] }) {
  return (
    <section {...sx(styles.panel)}>
      <div {...sx(styles.panelHeader)}>
        <h2 {...sx(styles.panelTitle)}>{props.title}</h2>
        <span {...sx(styles.subtle)}>{props.subtitle}</span>
      </div>
      <Show when={props.items.length > 0} fallback={<div {...sx(styles.empty)}>No data in this range yet.</div>}>
        <div {...sx(styles.breakdown)}>
          <For each={props.items}>
            {(item) => (
              <div {...sx(styles.breakdownRow)}>
                <span {...sx(styles.breakdownName)} title={item.name}>{item.name}</span>
                <span {...sx(styles.breakdownCount)}>{formatNumber(item.count)}</span>
              </div>
            )}
          </For>
        </div>
      </Show>
    </section>
  );
}

function formatNumber(value: number) {
  return new Intl.NumberFormat().format(value);
}
