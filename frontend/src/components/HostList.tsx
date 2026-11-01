import { useEffect, useRef } from "react"
import type { HostGroup } from "../lib/store"
import { processesOf } from "../lib/store"
import type { Me } from "../lib/types"
import { coords, count, duration, endpoint, place, scopeLabel, shortPlace, stateLabel } from "../lib/format"
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
                        </div>
                        <div className="right num">
                            {g.conns.length > 1 ? `${g.conns.length}×` : ""}
                            <div>{duration(now - g.since)}</div>
                        </div>
                        {isSel && (
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
                        )}
                    </div>
                )
            })}
        </div>
    )
}
