import { useMemo, useRef, useState } from "react"
import { useVirtualizer } from "@tanstack/react-virtual"
import { ArrowDown, ArrowUp, Search } from "lucide-react"
import { useStore } from "../lib/store"
import type { Conn, Host, Listener } from "../lib/types"
import { duration, endpoint, scopeLabel, shortPlace, stateLabel } from "../lib/format"

type SortKey = "process" | "remote" | "place" | "org" | "service" | "dir" | "since"

const COLS = "minmax(130px,1.1fr) 52px minmax(170px,1.5fr) minmax(130px,1.1fr) minmax(120px,1fr) 110px 86px 80px"
const PORT_COLS = "minmax(140px,1.2fr) 60px 90px minmax(130px,1fr) minmax(170px,1.4fr) 100px"

export function ConnectionsPage() {
    const { snap, settings, setSettings } = useStore()
    const [view, setView] = useState<"conns" | "ports">("conns")
    const [q, setQ] = useState("")
    const [sort, setSort] = useState<{ key: SortKey; asc: boolean }>({ key: "since", asc: false })
    const showLocal = !!settings?.showLocal

    const rows = useMemo(() => {
        const hosts = snap?.hosts ?? {}
        const words = q.toLowerCase().split(/\s+/).filter(Boolean)
        let list = (snap?.conns ?? []).filter(c => showLocal || c.scope === "public")
        if (words.length) {
            list = list.filter(c => {
                const h = hosts[c.remoteIp]
                const hay = [c.process, c.exe, c.remoteIp, c.remotePort, c.localPort, c.service, c.proto, h?.name, h?.city, h?.region, h?.country, h?.countryCode, h?.org, c.dir === "in" ? "incoming" : "outgoing"]
                    .join(" ")
                    .toLowerCase()
                return words.every(w => hay.includes(w))
            })
        }
        const val = (c: Conn): string | number => {
            const h = hosts[c.remoteIp]
            switch (sort.key) {
                case "process":
                    return (c.process || "~").toLowerCase()
                case "remote":
                    return (h?.name || c.remoteIp).toLowerCase()
                case "place":
                    return (h?.country || "~") + (h?.city || "")
                case "org":
                    return (h?.org || "~").toLowerCase()
                case "service":
                    return (c.service || "~") + c.remotePort
                case "dir":
                    return c.dir
                default:
                    return c.since
            }
        }
        return [...list].sort((a, b) => {
            const x = val(a)
            const y = val(b)
            const r = x < y ? -1 : x > y ? 1 : 0
            return sort.asc ? r : -r
        })
    }, [snap, q, sort, showLocal])

    const ports = useMemo(() => {
        const words = q.toLowerCase().split(/\s+/).filter(Boolean)
        return (snap?.listeners ?? []).filter(l => {
            if (!words.length) return true
            const hay = [l.process, l.exe, l.port, l.proto, l.service, l.ip, l.scope].join(" ").toLowerCase()
            return words.every(w => hay.includes(w))
        })
    }, [snap, q])

    const head = (key: SortKey, label: string) => (
        <button className={sort.key === key ? "on" : ""} onClick={() => setSort(s => ({ key, asc: s.key === key ? !s.asc : key !== "since" }))}>
            {label}
            {sort.key === key && (sort.asc ? <ArrowUp size={12} /> : <ArrowDown size={12} />)}
        </button>
    )

    return (
        <div className="page">
            <div className="page-head">
                <h1>Connections</h1>
                <div className="seg">
                    <button className={view === "conns" ? "on" : ""} onClick={() => setView("conns")}>
                        Open connections
                    </button>
                    <button className={view === "ports" ? "on" : ""} onClick={() => setView("ports")}>
                        Open ports
                    </button>
                </div>
                <div className="grow" />
                {view === "conns" && settings && (
                    <label className="check" title="Also list connections inside this computer and on your local network">
                        <button className={"switch" + (showLocal ? " on" : "")} onClick={() => setSettings({ ...settings, showLocal: !showLocal })} />
                        Local too
                    </label>
                )}
                <div className="search">
                    <Search size={15} />
                    <input className="input" placeholder="Filter: program, address, country, port…" value={q} onChange={e => setQ(e.target.value)} />
                </div>
            </div>
            {view === "conns" ? (
                <div className="table">
                    <div className="thead" style={{ gridTemplateColumns: COLS }}>
                        {head("process", "Program")}
                        <span>Proto</span>
                        {head("remote", "Remote")}
                        {head("place", "Location")}
                        {head("org", "Network")}
                        {head("service", "Service")}
                        {head("dir", "Direction")}
                        {head("since", "Open for")}
                    </div>
                    <ConnRows rows={rows} hosts={snap?.hosts ?? {}} />
                </div>
            ) : (
                <div className="table">
                    <div className="thead" style={{ gridTemplateColumns: PORT_COLS }}>
                        <span>Program</span>
                        <span>Proto</span>
                        <span>Port</span>
                        <span>Service</span>
                        <span>Reachable from</span>
                        <span>Open for</span>
                    </div>
                    <div className="scroll">
                        {ports.length ? ports.map(l => <PortRow key={l.proto + l.ip + l.port} l={l} />) : <div className="empty">No open ports.</div>}
                    </div>
                </div>
            )}
        </div>
    )
}

function ConnRows({ rows, hosts }: { rows: Conn[]; hosts: Record<string, Host> }) {
    const parent = useRef<HTMLDivElement>(null)
    const v = useVirtualizer({ count: rows.length, getScrollElement: () => parent.current, estimateSize: () => 44, overscan: 12 })
    const now = Date.now()
    if (!rows.length) return <div className="empty">No connections match.</div>
    return (
        <div className="scroll" ref={parent}>
            <div style={{ height: v.getTotalSize(), position: "relative" }}>
                {v.getVirtualItems().map(item => {
                    const c = rows[item.index]
                    const h = hosts[c.remoteIp]
                    return (
                        <div key={c.id} className="trow selectable" style={{ gridTemplateColumns: COLS, position: "absolute", top: 0, left: 0, right: 0, transform: `translateY(${item.start}px)` }}>
                            <div className="cell2">
                                <span className="a">{c.process || <span className="subtle">Unknown</span>}</span>
                                <span className="b">{c.pid ? `PID ${c.pid}` : "no access"}</span>
                            </div>
                            <span className="dim">{c.proto.toUpperCase()}</span>
                            <div className="cell2">
                                <span className="a">{h?.name || c.remoteIp}</span>
                                <span className="b mono">{endpoint(c.remoteIp, c.remotePort)}</span>
                            </div>
                            <div className="cell2">
                                <span className="a">{shortPlace(h) || <span className="subtle">{scopeLabel(c.scope)}</span>}</span>
                                <span className="b">{h?.region && h.region !== h.city ? h.region : ""}</span>
                            </div>
                            <div className="cell2">
                                <span className="a">{h?.org || <span className="subtle">—</span>}</span>
                                <span className="b">{h?.asn ? `AS${h.asn}` : ""}</span>
                            </div>
                            <div className="cell2">
                                <span className="a">{c.service || `Port ${c.dir === "in" ? c.localPort : c.remotePort}`}</span>
                                <span className="b">{stateLabel(c)}</span>
                            </div>
                            <span className={c.dir === "in" ? "k-open in" : "dim"}>{c.dir === "in" ? "Incoming" : "Outgoing"}</span>
                            <span className="num dim">{duration(now - c.since)}</span>
                        </div>
                    )
                })}
            </div>
        </div>
    )
}

function PortRow({ l }: { l: Listener }) {
    const reach = l.scope === "public" ? "Every network" : l.scope === "loopback" ? "This computer only" : `Local network (${l.ip})`
    return (
        <div className="trow selectable" style={{ gridTemplateColumns: PORT_COLS }}>
            <div className="cell2">
                <span className="a">{l.process || <span className="subtle">Unknown</span>}</span>
                <span className="b">{l.pid ? `PID ${l.pid}` : "no access"}</span>
            </div>
            <span className="dim">{l.proto.toUpperCase()}</span>
            <span className="num">{l.port}</span>
            <span className="ellipsis">{l.service || <span className="subtle">—</span>}</span>
            <span className={l.scope === "public" ? "" : "dim"}>{reach}</span>
            <span className="num dim">{duration(Date.now() - l.since)}</span>
        </div>
    )
}
