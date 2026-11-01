import { createContext, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react"
import { api, onStatus, onTick } from "./api"
import type { GeoStatus, Info, LogEvent, Settings, Snapshot, UpdateState } from "./types"

const LIVE_MAX = 400

export interface Store {
    snap: Snapshot | null
    events: LogEvent[] // newest last
    geo: GeoStatus | null
    update: UpdateState | null
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
        })
        const off1 = onTick(t => {
            setSnap(t.snapshot)
            addEvents(t.events)
            setGeo(t.geo)
            setUpdate(t.update)
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
        [snap, events, geo, update, info, settings, focus, selected],
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
    host: import("./types").Host | undefined
    conns: import("./types").Conn[]
    dir: "in" | "out"
    since: number
    scope: string
}

export function groupHosts(snap: Snapshot | null, includeLocal: boolean): HostGroup[] {
    if (!snap?.conns) return []
    const by = new Map<string, HostGroup>()
    for (const c of snap.conns) {
        if (!includeLocal && c.scope !== "public") continue
        let g = by.get(c.remoteIp)
        if (!g) {
            g = { ip: c.remoteIp, host: snap.hosts?.[c.remoteIp], conns: [], dir: "out", since: c.since, scope: c.scope }
            by.set(c.remoteIp, g)
        }
        g.conns.push(c)
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
