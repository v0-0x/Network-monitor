import { useEffect, useRef, useState } from "react"
import { Activity } from "lucide-react"
import type { HostGroup } from "../lib/store"
import { latency, processesOf } from "../lib/store"
import type { Me } from "../lib/types"
import { api } from "../lib/api"
import { bytes, bytesRate, coords, count, duration, endpoint, ms, place, riskName, scopeLabel, shortPlace, stateLabel } from "../lib/format"
import { distanceKm } from "./GlobeView"

interface Props {
    groups: HostGroup[]
    me: Me | null
    focus: string | null
    selected: string | null
    onHover: (ip: string | null) => void
    onSelect: (ip: string | null) => void
}

export function HostList({ groups, me, focus, selected, onHover, onSelect }: Props) {
    const selRef = useRef<HTMLDivElement>(null)
    useEffect(() => {
        selRef.current?.scrollIntoView({ block: "nearest" })
    }, [selected])
    if (!groups.length) return <div className="empty">No connections to the internet right now.</div>
    const now = Date.now()
    return (
        <div>
            {groups.map(g => {
                const h = g.host
                const isSel = g.ip === selected
                const cc = h?.countryCode || (g.scope === "public" ? "··" : "LAN")
                const rate = g.rxRate + g.txRate
                const lvl = h?.risk?.level
                return (
                    <div
                        key={g.ip}
                        ref={isSel ? selRef : undefined}
                        className={"host-row" + (isSel ? " sel" : "") + (g.ip === focus ? " hl" : "")}
                        onMouseEnter={() => onHover(g.ip)}
                        onMouseLeave={() => onHover(null)}
                        onClick={() => onSelect(isSel ? null : g.ip)}
                    >
                        <span className={"badge" + (g.dir === "in" ? " in" : "")} title={h?.country || scopeLabel(g.scope)}>
                            {cc}
                        </span>
                        <div className="main">
                            <div className="title ellipsis">{h?.name || h?.org || g.ip}</div>
                            <div className="sub ellipsis">
                                {processesOf(g)}
                                {shortPlace(h) ? ` · ${shortPlace(h)}` : g.scope !== "public" ? ` · ${scopeLabel(g.scope)}` : ""}
                            </div>
                            {(lvl === "medium" || lvl === "high") && <span className={"risk-tag " + lvl}>{riskName[lvl]} risk</span>}
                        </div>
                        <div className="right num">
                            {rate >= 1000 ? <div className="dim">{bytesRate(rate)}</div> : g.conns.length > 1 ? `${g.conns.length}×` : ""}
                            <div>{duration(now - g.since)}</div>
                        </div>
                        {isSel && <HostDetail g={g} me={me} />}
                    </div>
                )
            })}
        </div>
    )
}

function HostDetail({ g, me }: { g: HostGroup; me: Me | null }) {
    const h = g.host
    const [pinging, setPinging] = useState(false)
    const [pingErr, setPingErr] = useState("")
    const lat = latency(g)
    const p = h?.ping
    const doPing = async () => {
        setPinging(true)
        setPingErr("")
        try {
            const r = await api.pingHost(g.ip)
            if (!r.received) setPingErr(r.error || "No answer")
        } catch (e) {
            setPingErr(String(e))
        } finally {
            setPinging(false)
        }
    }
    return (
        <div className="host-detail selectable" onClick={e => e.stopPropagation()}>
            <dl className="kv">
                <dt>Address</dt>
                <dd className="mono">{g.ip}</dd>
                {h?.name && (
                    <>
                        <dt>Host name</dt>
                        <dd>{h.name}</dd>
                    </>
                )}
                {place(h) && (
                    <>
                        <dt>Location</dt>
                        <dd>{place(h)}</dd>
                    </>
                )}
                {h?.hasCoords && (
                    <>
                        <dt>Coordinates</dt>
                        <dd>{coords(h.lat, h.lon)}</dd>
                    </>
                )}
                {(h?.org || h?.asn) && (
                    <>
                        <dt>Network</dt>
                        <dd>
                            {h?.org} {h?.asn ? <span className="subtle">AS{h.asn}</span> : null}
                        </dd>
                    </>
                )}
                {h?.timeZone && (
                    <>
                        <dt>Time zone</dt>
                        <dd>{h.timeZone}</dd>
                    </>
                )}
                {h?.hasCoords && me?.known && (
                    <>
                        <dt>Distance</dt>
                        <dd>{count(Math.round(distanceKm(me.lat, me.lon, h.lat, h.lon)))} km</dd>
                    </>
                )}
                {g.hasBytes && (
                    <>
                        <dt>Traffic</dt>
                        <dd>
                            ↓ {bytes(g.rx)} ↑ {bytes(g.tx)}
                            {g.rxRate + g.txRate > 0 && (
                                <div className="subtle">
                                    now ↓ {bytesRate(g.rxRate)} ↑ {bytesRate(g.txRate)}
                                </div>
                            )}
                        </dd>
                    </>
                )}
                <dt>Latency</dt>
                <dd>
                    <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                        <span>
                            {ms(lat)}
                            {p && p.received > 0 && (
                                <span className="subtle">
                                    {" "}
                                    {p.method === "icmp" ? "ping" : "TCP"} {ms(p.min)}–{ms(p.max)}
                                    {p.sent > p.received ? `, ${Math.round(((p.sent - p.received) / p.sent) * 100)}% lost` : ""}
                                </span>
                            )}
                            {!(p && p.received > 0) && g.rtt !== undefined && <span className="subtle"> connection</span>}
                        </span>
                        <button className="btn small" onClick={doPing} disabled={pinging} title="Ping it now">
                            <Activity size={13} />
                            {pinging ? "Pinging…" : "Ping"}
                        </button>
                    </div>
                    {pingErr && <div className="subtle">{pingErr}</div>}
                </dd>
                {h?.risk && (
                    <>
                        <dt>Risk</dt>
                        <dd>
                            <span className={"risk " + h.risk.level}>
                                {riskName[h.risk.level]} · {h.risk.score}
                            </span>
                            {h.risk.reasons.length > 0 && (
                                <ul className="why">
                                    {h.risk.reasons.map(r => (
                                        <li key={r}>{r}</li>
                                    ))}
                                </ul>
                            )}
                        </dd>
                    </>
                )}
                <dt>Connections</dt>
                <dd>
                    {g.conns.map(c => (
                        <div key={c.id}>
                            {c.process || "Unknown"}
                            {c.pid ? <span className="subtle"> ({c.pid})</span> : null} · {c.proto.toUpperCase()} {endpoint(c.localIp, c.localPort)} {c.dir === "in" ? "←" : "→"} {c.remotePort}
                            {c.service ? ` ${c.service}` : ""} <span className="subtle">· {stateLabel(c)}</span>
                        </div>
                    ))}
                </dd>
            </dl>
        </div>
    )
}
