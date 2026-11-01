import { createContext, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react"
import { api, onStatus, onTick } from "./api"
import type { BlocklistStatus, Conn, GeoStatus, Host, Info, LogEvent, Settings, Snapshot, UpdateState } from "./types"

const LIVE_MAX = 400

export interface Store {
    snap: Snapshot | null
    events: LogEvent[] // newest last
    geo: GeoStatus | null
    update: UpdateState | null
    blocklists: BlocklistStatus | null
    info: Info | null
    settings: Settings | null
    setSettings: (s: Settings) => Promise<void>
    // The host hovered or picked anywhere, so the globe and lists agree.
    focus: string | null
    setFocus: (ip: string | null) => void
    selected: string | null
    select: (ip: string | null) => void
}

const Ctx = createContext<Store | null>(null)

export function StoreProvider({ children }: { children: ReactNode }) {
    const [snap, setSnap] = useState<Snapshot | null>(null)
    const [events, setEvents] = useState<LogEvent[]>([])
    const [geo, setGeo] = useState<GeoStatus | null>(null)
    const [update, setUpdate] = useState<UpdateState | null>(null)
    const [blocklists, setBlocklists] = useState<BlocklistStatus | null>(null)
    const [info, setInfo] = useState<Info | null>(null)
    const [settings, setSettingsState] = useState<Settings | null>(null)
    const [focus, setFocus] = useState<string | null>(null)
    const [selected, select] = useState<string | null>(null)
    const lastId = useRef(0)

    useEffect(() => {
        const addEvents = (es: LogEvent[] | null) => {
            if (!es?.length) return
            const fresh = es.filter(e => e.id > lastId.current)
            if (!fresh.length) return
            lastId.current = fresh[fresh.length - 1].id
            setEvents(old => {
                const next = old.concat(fresh)
                return next.length > LIVE_MAX ? next.slice(next.length - LIVE_MAX) : next
            })
        }
        api.info().then(setInfo)
        api.settings().then(setSettingsState)
        api.tick().then(t => {
            if (t.snapshot?.time) setSnap(t.snapshot)
            addEvents(t.events)
            setGeo(t.geo)
            setUpdate(t.update)
            setBlocklists(t.blocklists)
        })
        const off1 = onTick(t => {
            setSnap(t.snapshot)
            addEvents(t.events)
            setGeo(t.geo)
            setUpdate(t.update)
            setBlocklists(t.blocklists)
        })
        const off2 = onStatus(s => {
            setGeo(s.geo)
            setUpdate(s.update)
        })
        return () => {
            off1()
            off2()
        }
    }, [])

    const value = useMemo<Store>(
        () => ({
            snap,
            events,
            geo,
            update,
            blocklists,
            info,
            settings,
            setSettings: async (s: Settings) => {
                const saved = await api.saveSettings(s)
                setSettingsState(saved)
            },
            focus,
            setFocus,
            selected,
            select,
        }),
        [snap, events, geo, update, blocklists, info, settings, focus, selected],
    )
    return <Ctx.Provider value={value}>{children}</Ctx.Provider>
}

export function useStore(): Store {
    const s = useContext(Ctx)
    if (!s) throw new Error("useStore outside StoreProvider")
    return s
}

// Hosts with their connections, newest first.
export interface HostGroup {
    ip: string
    host: Host | undefined
    conns: Conn[]
    dir: "in" | "out"
    since: number
    scope: string
    hasBytes: boolean
    rx: number
    tx: number
    rxRate: number
    txRate: number
    rtt?: number // the lowest of its connections' round-trip times
}

export function groupHosts(snap: Snapshot | null, includeLocal: boolean): HostGroup[] {
    if (!snap?.conns) return []
    const by = new Map<string, HostGroup>()
    for (const c of snap.conns) {
        if (!includeLocal && c.scope !== "public") continue
        let g = by.get(c.remoteIp)
        if (!g) {
            g = { ip: c.remoteIp, host: snap.hosts?.[c.remoteIp], conns: [], dir: "out", since: c.since, scope: c.scope, hasBytes: false, rx: 0, tx: 0, rxRate: 0, txRate: 0 }
            by.set(c.remoteIp, g)
        }
        g.conns.push(c)
        if (c.hasBytes) {
            g.hasBytes = true
            g.rx += c.rx
            g.tx += c.tx
            g.rxRate += c.rxRate
            g.txRate += c.txRate
        }
        if (c.rtt && (!g.rtt || c.rtt < g.rtt)) g.rtt = c.rtt
        if (c.dir === "in") g.dir = "in"
        if (c.since < g.since) g.since = c.since
    }
    return [...by.values()].sort((a, b) => {
        const la = Math.max(...a.conns.map(c => c.since))
        const lb = Math.max(...b.conns.map(c => c.since))
        return lb - la || a.ip.localeCompare(b.ip)
    })
}

export function processesOf(g: HostGroup): string {
    const names = [...new Set(g.conns.map(c => c.process || "unknown"))]
    return names.slice(0, 3).join(", ") + (names.length > 3 ? ` +${names.length - 3}` : "")
}

// latency is the best number for how far away a host is: its last ping, or
// the system's round-trip time of the connection.
export function latency(g: Pick<HostGroup, "host" | "rtt">): number | undefined {
    const p = g.host?.ping
    if (p && p.received > 0) return p.avg
    return g.rtt
}

export interface ProgramGroup {
    name: string
    exe?: string
    pids: number[]
    conns: number
    hosts: number
    countries: number
    hasBytes: boolean
    rx: number
    tx: number
    rxRate: number
    txRate: number
    risk: number
}

// Programs with their connections, busiest first.
export function groupPrograms(snap: Snapshot | null, includeLocal: boolean): ProgramGroup[] {
    if (!snap?.conns) return []
    const by = new Map<string, ProgramGroup & { _hosts: Set<string>; _cc: Set<string> }>()
    for (const c of snap.conns) {
        if (!includeLocal && c.scope !== "public") continue
        const name = c.process || "Unknown"
        let g = by.get(name)
        if (!g) {
            g = { name, exe: c.exe, pids: [], conns: 0, hosts: 0, countries: 0, hasBytes: false, rx: 0, tx: 0, rxRate: 0, txRate: 0, risk: 0, _hosts: new Set(), _cc: new Set() }
            by.set(name, g)
        }
        g.conns++
        if (c.pid && !g.pids.includes(c.pid)) g.pids.push(c.pid)
        g._hosts.add(c.remoteIp)
        const h = snap.hosts?.[c.remoteIp]
        if (h?.countryCode) g._cc.add(h.countryCode)
        if (h?.risk && h.risk.score > g.risk) g.risk = h.risk.score
        if (c.hasBytes) {
            g.hasBytes = true
            g.rx += c.rx
            g.tx += c.tx
            g.rxRate += c.rxRate
            g.txRate += c.txRate
        }
    }
    return [...by.values()]
        .map(({ _hosts, _cc, ...g }) => ({ ...g, hosts: _hosts.size, countries: _cc.size }))
        .sort((a, b) => b.rxRate + b.txRate - (a.rxRate + a.txRate) || b.rx + b.tx - (a.rx + a.tx) || b.conns - a.conns)
}
