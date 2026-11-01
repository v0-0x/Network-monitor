import { useMemo, useRef, useState } from "react"
import { useVirtualizer } from "@tanstack/react-virtual"
import { ArrowDown, ArrowUp, Search } from "lucide-react"
import { groupPrograms, useStore, type ProgramGroup } from "../lib/store"
import type { Conn, Host, Listener } from "../lib/types"
import { bytes, bytesRate, count, duration, endpoint, ms, riskLevel, riskName, scopeLabel, shortPlace, stateLabel } from "../lib/format"

type SortKey = "process" | "remote" | "place" | "org" | "service" | "traffic" | "rate" | "latency" | "risk" | "since"

const COLS = "minmax(120px,1fr) minmax(160px,1.4fr) minmax(120px,1fr) minmax(120px,1fr) minmax(110px,0.9fr) 132px 66px 78px 70px"
const PORT_COLS = "minmax(140px,1.2fr) 60px 90px minmax(130px,1fr) minmax(170px,1.4fr) 100px"
const PROG_COLS = "minmax(160px,1.4fr) 92px 60px 76px 100px 100px 180px 110px"

// Ports that shouldn't be open to every network unless you mean it.
const EXPOSED = new Set([21, 22, 23, 139, 445, 1433, 2375, 3306, 3389, 5432, 5900, 5985, 6379, 9200, 11211, 27017])

export function ConnectionsPage() {
    const { snap, settings, setSettings } = useStore()
    const [view, setView] = useState<"conns" | "programs" | "ports">("conns")
    const [q, setQ] = useState("")
    const [sort, setSort] = useState<{ key: SortKey; asc: boolean }>({ key: "since", asc: false })
    const showLocal = !!settings?.showLocal
    const words = useMemo(() => q.toLowerCase().split(/\s+/).filter(Boolean), [q])

    const rows = useMemo(() => {
        const hosts = snap?.hosts ?? {}
        let list = (snap?.conns ?? []).filter(c => showLocal || c.scope === "public")
        if (words.length) {
            list = list.filter(c => {
                const h = hosts[c.remoteIp]
                const hay = [c.process, c.exe, c.remoteIp, c.remotePort, c.localPort, c.service, c.proto, h?.name, h?.city, h?.region, h?.country, h?.countryCode, h?.org, c.dir === "in" ? "incoming" : "outgoing", h?.risk?.level ? h.risk.level + " risk" : ""]
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
                case "traffic":
                    return c.rx + c.tx
                case "rate":
                    return c.rxRate + c.txRate
                case "latency":
                    return (h?.ping?.received ? h.ping.avg : c.rtt) ?? 1e9
                case "risk":
                    return h?.risk?.score ?? 0
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
    }, [snap, words, sort, showLocal])

    const programs = useMemo(() => {
        const list = groupPrograms(snap, showLocal)
        if (!words.length) return list
        return list.filter(p => words.every(w => [p.name, p.exe, p.pids.join(" ")].join(" ").toLowerCase().includes(w)))
    }, [snap, words, showLocal])

    const ports = useMemo(() => {
        return (snap?.listeners ?? []).filter(l => {
            if (!words.length) return true
            const hay = [l.process, l.exe, l.port, l.proto, l.service, l.ip, l.scope].join(" ").toLowerCase()
            return words.every(w => hay.includes(w))
        })
    }, [snap, words])

    // Numbers sort biggest first on the first click; words A to Z.
    const head = (key: SortKey, label: string) => (
        <button className={sort.key === key ? "on" : ""} onClick={() => setSort(s => ({ key, asc: s.key === key ? !s.asc : ["process", "remote", "place", "org", "service", "latency"].includes(key) }))}>
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
                    <button className={view === "programs" ? "on" : ""} onClick={() => setView("programs")}>
                        Programs
                    </button>
                    <button className={view === "ports" ? "on" : ""} onClick={() => setView("ports")}>
                        Open ports
                    </button>
                </div>
                <div className="grow" />
                {view !== "ports" && settings && (
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
            {snap && !snap.perConn && view !== "ports" && (
                <div className="subtle" style={{ padding: "8px 18px", borderBottom: "1px solid var(--line)", fontSize: 12 }}>
                    {snap.perConnNote}
                </div>
            )}
            {view === "conns" && (
                <div className="table">
                    <div className="thead" style={{ gridTemplateColumns: COLS }}>
                        {head("process", "Program")}
                        {head("remote", "Remote")}
                        {head("place", "Location")}
                        {head("org", "Network")}
                        {head("service", "Service")}
                        {head("traffic", "Traffic")}
                        {head("latency", "Latency")}
                        {head("risk", "Risk")}
                        {head("since", "Open for")}
                    </div>
                    <ConnRows rows={rows} hosts={snap?.hosts ?? {}} />
                </div>
            )}
            {view === "programs" && (
                <div className="table">
                    <div className="thead" style={{ gridTemplateColumns: PROG_COLS }}>
                        <span>Program</span>
                        <span>Connections</span>
                        <span>Hosts</span>
                        <span>Countries</span>
                        <span>Downloaded</span>
                        <span>Uploaded</span>
                        <span>Speed now</span>
                        <span>Highest risk</span>
                    </div>
                    <div className="scroll">{programs.length ? programs.map(p => <ProgramRow key={p.name} p={p} />) : <div className="empty">No programs match.</div>}</div>
                </div>
            )}
            {view === "ports" && (
                <div className="table">
                    <div className="thead" style={{ gridTemplateColumns: PORT_COLS }}>
                        <span>Program</span>
                        <span>Proto</span>
                        <span>Port</span>
                        <span>Service</span>
                        <span>Reachable from</span>
                        <span>Open for</span>
                    </div>
                    <div className="scroll">{ports.length ? ports.map(l => <PortRow key={l.proto + l.ip + l.port} l={l} />) : <div className="empty">No open ports.</div>}</div>
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
                    const lat = h?.ping?.received ? h.ping.avg : c.rtt
                    const rate = c.rxRate + c.txRate
                    return (
                        <div key={c.id} className="trow selectable" style={{ gridTemplateColumns: COLS, position: "absolute", top: 0, left: 0, right: 0, transform: `translateY(${item.start}px)` }}>
                            <div className="cell2">
                                <span className="a">{c.process || <span className="subtle">Unknown</span>}</span>
                                <span className="b">{c.pid ? `PID ${c.pid}` : "no access"}</span>
                            </div>
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
                                <span className={"b" + (c.dir === "in" ? " k-open in" : "")}>
                                    {c.proto.toUpperCase()} · {c.dir === "in" ? "Incoming" : "Outgoing"} · {stateLabel(c)}
                                </span>
                            </div>
                            <div className="cell2 num">
                                <span className="a">{c.hasBytes ? `↓ ${bytes(c.rx)} ↑ ${bytes(c.tx)}` : <span className="subtle">—</span>}</span>
                                <span className="b">{c.hasBytes && rate > 0 ? bytesRate(rate) : ""}</span>
                            </div>
                            <span className="num dim">{ms(lat)}</span>
                            <span>{h?.risk && c.scope === "public" ? <span className={"risk " + h.risk.level} title={h.risk.reasons.join("\n")}>{riskName[h.risk.level]}</span> : <span className="subtle">—</span>}</span>
                            <span className="num dim">{duration(now - c.since)}</span>
                        </div>
                    )
                })}
            </div>
        </div>
    )
}

function ProgramRow({ p }: { p: ProgramGroup }) {
    const lvl = riskLevel(p.risk)
    return (
        <div className="trow selectable" style={{ gridTemplateColumns: PROG_COLS }}>
            <div className="cell2">
                <span className="a">{p.name}</span>
                <span className="b">{p.exe || (p.pids.length ? `PID ${p.pids.slice(0, 3).join(", ")}` : "no access")}</span>
            </div>
            <span className="num">{count(p.conns)}</span>
            <span className="num">{count(p.hosts)}</span>
            <span className="num">{count(p.countries)}</span>
            <span className="num">{p.hasBytes ? bytes(p.rx) : <span className="subtle">—</span>}</span>
            <span className="num">{p.hasBytes ? bytes(p.tx) : <span className="subtle">—</span>}</span>
            <span className="num dim">{p.hasBytes ? `↓ ${bytesRate(p.rxRate)} ↑ ${bytesRate(p.txRate)}` : "—"}</span>
            <span className={"risk " + lvl}>
                {riskName[lvl]} · {p.risk}
            </span>
        </div>
    )
}

function PortRow({ l }: { l: Listener }) {
    const reach = l.scope === "public" ? "Every network" : l.scope === "loopback" ? "This computer only" : `Local network (${l.ip})`
    const exposed = l.scope === "public" && EXPOSED.has(l.port)
    return (
        <div className="trow selectable" style={{ gridTemplateColumns: PORT_COLS }}>
            <div className="cell2">
                <span className="a">{l.process || <span className="subtle">Unknown</span>}</span>
                <span className="b">{l.pid ? `PID ${l.pid}` : "no access"}</span>
            </div>
            <span className="dim">{l.proto.toUpperCase()}</span>
            <span className="num">{l.port}</span>
            <span className="ellipsis">{l.service || <span className="subtle">—</span>}</span>
            <span className={l.scope === "public" ? "" : "dim"} title={exposed ? "Open to every network: if your router forwards this port, the internet can reach it" : undefined}>
                {reach}
                {exposed && <span className="risk-tag medium" style={{ marginLeft: 8 }}>Check this</span>}
            </span>
            <span className="num dim">{duration(Date.now() - l.since)}</span>
        </div>
    )
}
