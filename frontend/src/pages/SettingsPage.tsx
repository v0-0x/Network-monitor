import { useEffect, useState, type ReactNode } from "react"
import { api } from "../lib/api"
import { useStore } from "../lib/store"
import type { Settings } from "../lib/types"
import { bytes, coords, dbVersion } from "../lib/format"

export function SettingsPage() {
    const { settings, setSettings, snap, geo, update, info, blocklists } = useStore()
    const [loc, setLoc] = useState(settings?.location)
    const [locErr, setLocErr] = useState("")
    const [checking, setChecking] = useState(false)
    useEffect(() => setLoc(settings?.location), [settings?.location])
    if (!settings || !loc) return <div className="page" />

    const save = (patch: Partial<Settings>) => setSettings({ ...settings, ...patch })
    const me = snap?.me
    const saveLocation = () => {
        const lat = Number(loc.lat)
        const lon = Number(loc.lon)
        if (loc.mode === "manual" && (!isFinite(lat) || !isFinite(lon) || Math.abs(lat) > 90 || Math.abs(lon) > 180)) {
            setLocErr("Latitude goes from -90 to 90 and longitude from -180 to 180.")
            return
        }
        setLocErr("")
        save({ location: { ...loc, lat, lon } })
    }
    const locDirty = JSON.stringify(loc) !== JSON.stringify(settings.location)

    return (
        <div className="page">
            <div className="page-head">
                <h1>Settings</h1>
            </div>
            <div className="scroll">
                <div className="settings">
                    <section className="card">
                        <h2>Your location</h2>
                        <div className="desc">Where the lines on the globe end.</div>
                        <div className="seg" style={{ justifySelf: "start" }}>
                            <button className={loc.mode === "auto" ? "on" : ""} onClick={() => setLoc({ ...loc, mode: "auto" })}>
                                From my IP address
                            </button>
                            <button className={loc.mode === "manual" ? "on" : ""} onClick={() => setLoc({ ...loc, mode: "manual" })}>
                                Set by hand
                            </button>
                        </div>
                        {loc.mode === "auto" ? (
                            <div className="dim selectable">
                                {me?.known && !me.manual ? (
                                    <>
                                        {me.label || coords(me.lat, me.lon)}
                                        {me.ip && <span className="subtle"> · public IP {me.ip}</span>}
                                        {me.org && <span className="subtle"> · {me.org}</span>}
                                    </>
                                ) : (
                                    me?.error || "Looking it up…"
                                )}
                                <div className="subtle" style={{ marginTop: 4, fontSize: 12 }}>
                                    Found by asking a public service (ipify) for your address, then looking it up offline. With a VPN on, this is the VPN's location.
                                </div>
                            </div>
                        ) : (
                            <>
                                <div className="fields">
                                    <label>
                                        Latitude
                                        <input className="input" inputMode="decimal" value={loc.lat} onChange={e => setLoc({ ...loc, lat: e.target.value as unknown as number })} />
                                    </label>
                                    <label>
                                        Longitude
                                        <input className="input" inputMode="decimal" value={loc.lon} onChange={e => setLoc({ ...loc, lon: e.target.value as unknown as number })} />
                                    </label>
                                    <label>
                                        Name (optional)
                                        <input className="input" placeholder="Home" value={loc.label} onChange={e => setLoc({ ...loc, label: e.target.value })} />
                                    </label>
                                </div>
                                {me?.known && !me.manual && (
                                    <div>
                                        <button className="link" onClick={() => setLoc({ ...loc, lat: +me.lat.toFixed(4), lon: +me.lon.toFixed(4), label: me.label })}>
                                            Start from the detected place ({me.label})
                                        </button>
                                    </div>
                                )}
                            </>
                        )}
                        {locErr && <div className="err">{locErr}</div>}
                        {locDirty && (
                            <div>
                                <button className="btn primary" onClick={saveLocation}>
                                    Save location
                                </button>
                            </div>
                        )}
                    </section>

                    <section className="card">
                        <h2>Monitoring</h2>
                        <Row label="Check connections every" hint="More often catches shorter connections and uses a little more CPU.">
                            <select className="select" value={settings.intervalMs} onChange={e => save({ intervalMs: Number(e.target.value) })}>
                                <option value={500}>0.5 seconds</option>
                                <option value={1000}>1 second</option>
                                <option value={2000}>2 seconds</option>
                                <option value={5000}>5 seconds</option>
                            </select>
                        </Row>
                        <Row label="Look up host names" hint="Asks your DNS server for each address's name (reverse DNS), like 'lhr25s34-in-f14.1e100.net'.">
                            <Switch on={settings.resolveNames} set={v => save({ resolveNames: v })} />
                        </Row>
                        <Row label="Include local connections" hint="Also list and log connections inside this computer and to your home network.">
                            <Switch on={settings.showLocal} set={v => save({ showLocal: v })} />
                        </Row>
                        <Row
                            label="Ping hosts"
                            hint={snap && !snap.icmp ? snap.icmpNote || "Ping (ICMP) isn't allowed here." : "Pings every host on the internet when it's first seen and then once a minute (3 small packets), for its latency."}
                        >
                            <Switch on={settings.pingHosts} set={v => save({ pingHosts: v })} />
                        </Row>
                        {snap && (
                            <div className="subtle" style={{ fontSize: 12 }}>
                                Traffic per connection: {snap.perConn ? "counted" : "not available"}
                                {snap.perConnNote ? `. ${snap.perConnNote}` : "."}
                            </div>
                        )}
                        {snap && (
                            <div className="subtle" style={{ fontSize: 12 }}>
                                Reading the connection table takes {snap.scanMs.toFixed(1)} ms ({snap.method}).
                                {snap.error && <span className="err"> {snap.error}</span>}
                            </div>
                        )}
                    </section>

                    <section className="card">
                        <h2>Risk checks</h2>
                        <div className="desc">
                            Each host gets a risk score from 0 to 100, with its reasons: being on a public blocklist, someone on the internet connecting in (more so to remote access like SSH or Remote Desktop), unencrypted protocols, ports malware likes, and programs running from a temporary or downloads folder. It's a hint where to look, not a verdict. Where an address is (its country) doesn't count.
                        </div>
                        <Row
                            label="Check addresses against blocklists"
                            hint={
                                blocklists?.lists?.some(l => l.entries > 0)
                                    ? `${blocklists.lists
                                          .filter(l => l.entries > 0)
                                          .map(l => `${l.name} (${l.entries.toLocaleString()})`)
                                          .join(", ")} · updated ${new Date(blocklists.updated).toLocaleString()}. Downloaded twice a day; nothing about your connections is sent.`
                                    : "FireHOL level 1 and abuse.ch's Feodo Tracker, downloaded twice a day; nothing about your connections is sent."
                            }
                        >
                            <Switch on={settings.blocklists} set={v => save({ blocklists: v })} />
                        </Row>
                        {blocklists?.error && <div className="err">{blocklists.error}</div>}
                        {settings.blocklists && (
                            <div>
                                <button className="btn" onClick={() => api.updateBlocklists()}>
                                    Update the blocklists now
                                </button>
                            </div>
                        )}
                    </section>

                    <section className="card">
                        <h2>Logs</h2>
                        <Row label="Keep logs for" hint="Older days are deleted.">
                            <select className="select" value={settings.logDays} onChange={e => save({ logDays: Number(e.target.value) })}>
                                {[3, 7, 14, 30, 90, 365].map(d => (
                                    <option key={d} value={d}>
                                        {d} days
                                    </option>
                                ))}
                            </select>
                        </Row>
                        <Row label="Logs folder" hint={info?.logsDir}>
                            <button className="btn" onClick={() => api.openLogsFolder()}>
                                Open
                            </button>
                        </Row>
                    </section>

                    <section className="card">
                        <h2>Location database</h2>
                        <div className="desc">
                            Addresses are placed on the map offline, from DB-IP's free database, so they're never sent anywhere to be looked up.{" "}
                            <button className="link" onClick={() => api.openURL("https://db-ip.com")}>
                                IP Geolocation by DB-IP
                            </button>{" "}
                            (CC BY 4.0).
                        </div>
                        {geo?.downloading ? (
                            <div style={{ display: "grid", gap: 6 }}>
                                <div className="dim">
                                    {geo.stage}: {bytes(geo.done)}
                                    {geo.total > 0 ? ` of ${bytes(geo.total)}` : ""}
                                </div>
                                {geo.total > 0 && (
                                    <div className="progress">
                                        <div style={{ width: `${(geo.done / geo.total) * 100}%` }} />
                                    </div>
                                )}
                            </div>
                        ) : (
                            <Row
                                label={geo?.ready ? `DB-IP Lite ${dbVersion(geo.cityVersion)}` : "Not downloaded yet"}
                                hint={geo?.ready ? `${bytes(geo.sizeBytes)} · downloaded ${new Date(geo.downloaded).toLocaleDateString()}${geo.hasAsn ? "" : " · no network names"}` : undefined}
                            >
                                <button className="btn" onClick={() => api.updateGeo()}>
                                    {geo?.ready ? "Check for a newer one" : "Download"}
                                </button>
                            </Row>
                        )}
                        {geo?.error && <div className="err">{geo.error}</div>}
                        <Row label="Update it every month" hint="DB-IP publishes a new one at the start of each month.">
                            <Switch on={settings.geoAuto} set={v => save({ geoAuto: v })} />
                        </Row>
                    </section>

                    <section className="card">
                        <h2>Updates</h2>
                        <div className="desc">
                            You have version {info?.version}
                            {info?.channel === "beta" ? " (beta)" : ""}.
                        </div>
                        {update?.manual ? (
                            <div className="dim selectable">{update.manual}</div>
                        ) : (
                            <>
                                <Row label="Look for updates by itself" hint="At start and every 6 hours.">
                                    <Switch on={settings.updates.auto} set={v => save({ updates: { ...settings.updates, auto: v } })} />
                                </Row>
                                <Row label="Beta updates" hint="Get new versions before they're marked stable.">
                                    <Switch on={update?.beta ?? false} set={v => save({ updates: { ...settings.updates, beta: v, betaChosen: true } })} />
                                </Row>
                                <UpdateBox checking={checking} />
                                <div>
                                    <button
                                        className="btn"
                                        disabled={checking || update?.downloading || update?.installing}
                                        onClick={async () => {
                                            setChecking(true)
                                            try {
                                                await api.checkForUpdates()
                                            } finally {
                                                setChecking(false)
                                            }
                                        }}
                                    >
                                        {checking ? "Checking…" : "Check now"}
                                    </button>
                                </div>
                            </>
                        )}
                        {update?.error && <div className="err">{update.error}</div>}
                    </section>

                    <section className="card">
                        <h2>About</h2>
                        <dl className="kv selectable">
                            <dt>Version</dt>
                            <dd>
                                {info?.version} {info?.channel} {info?.commit ? <span className="subtle mono">{info.commit.slice(0, 7)}</span> : null}
                            </dd>
                            <dt>System</dt>
                            <dd>
                                {info?.os} {info?.arch}
                            </dd>
                            <dt>Data folder</dt>
                            <dd className="mono">{info?.dataDir}</dd>
                            <dt>Source</dt>
                            <dd>
                                <button className="link" onClick={() => api.openURL("https://github.com/v0-0x/Network-monitor")}>
                                    github.com/v0-0x/Network-monitor
                                </button>
                            </dd>
                        </dl>
                    </section>
                </div>
            </div>
        </div>
    )
}

function UpdateBox({ checking }: { checking: boolean }) {
    const { update } = useStore()
    if (!update) return null
    const av = update.available
    if (!av) {
        return <div className="dim">{checking ? "Looking…" : update.lastCheck && !update.lastCheck.startsWith("0001") ? `Up to date (checked ${new Date(update.lastCheck).toLocaleTimeString()}).` : ""}</div>
    }
    return (
        <div style={{ display: "grid", gap: 10 }}>
            <div>
                <b>Version {av.version}</b>
                {av.beta ? " beta" : ""} is available.{" "}
                <button className="link" onClick={() => api.openURL(av.url)}>
                    Release page
                </button>
            </div>
            {av.notes && <div className="notes selectable">{av.notes}</div>}
            {update.downloading && update.total > 0 && (
                <div className="progress">
                    <div style={{ width: `${(update.done / update.total) * 100}%` }} />
                </div>
            )}
            <div style={{ display: "flex", gap: 8 }}>
                {update.ready ? (
                    <button className="btn primary" onClick={() => api.restartToUpdate()}>
                        Restart to update
                    </button>
                ) : (
                    <>
                        <button className="btn primary" disabled={update.downloading || update.installing} onClick={() => api.installUpdate()}>
                            {update.downloading ? `Downloading… ${update.total ? Math.round((update.done / update.total) * 100) + "%" : ""}` : update.installing ? "Installing…" : "Install update"}
                        </button>
                        <button className="btn ghost" onClick={() => api.skipVersion(av.version)}>
                            Skip this version
                        </button>
                    </>
                )}
            </div>
        </div>
    )
}

function Row({ label, hint, children }: { label: string; hint?: string; children: ReactNode }) {
    return (
        <div className="setting">
            <div className="label">
                <div>{label}</div>
                {hint && <div className="hint selectable">{hint}</div>}
            </div>
            {children}
        </div>
    )
}

function Switch({ on, set }: { on: boolean; set: (v: boolean) => void }) {
    return <button className={"switch" + (on ? " on" : "")} onClick={() => set(!on)} role="switch" aria-checked={on} />
}
