import { useEffect, useMemo, useRef, useState } from "react"
import { useVirtualizer } from "@tanstack/react-virtual"
import { Download, FolderOpen, Search } from "lucide-react"
import { api } from "../lib/api"
import { useStore } from "../lib/store"
import type { LogDay, LogEvent } from "../lib/types"
import { bytes, count, eventMatches } from "../lib/format"
import { EventRow } from "../components/EventRow"

const KINDS: { id: string; label: string; kinds: string[] }[] = [
    { id: "all", label: "All", kinds: [] },
    { id: "open", label: "Opened", kinds: ["open"] },
    { id: "close", label: "Closed", kinds: ["close"] },
    { id: "ports", label: "Ports", kinds: ["listen", "unlisten"] },
    { id: "notes", label: "Notes", kinds: ["note", "error"] },
]

const today = () => {
    const d = new Date()
    return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`
}

export function LogsPage({ active }: { active: boolean }) {
    const { events: live } = useStore()
    const [days, setDays] = useState<LogDay[]>([])
    const [day, setDay] = useState(today())
    const [text, setText] = useState("")
    const [kind, setKind] = useState("all")
    const [list, setList] = useState<LogEvent[]>([])
    const [more, setMore] = useState(false)
    const [total, setTotal] = useState(0)
    const [loading, setLoading] = useState(false)
    const [msg, setMsg] = useState("")
    const kinds = useMemo(() => KINDS.find(k => k.id === kind)!.kinds, [kind])
    const seenLive = useRef(0)
    const parent = useRef<HTMLDivElement>(null)

    useEffect(() => {
        if (active) api.logDays().then(d => setDays(d ?? []))
    }, [active])

    // Search again when the search changes (typing waits a moment).
    useEffect(() => {
        if (!active) return
        let gone = false
        const t = setTimeout(async () => {
            setLoading(true)
            try {
                const p = await api.searchLogs({ day, text, kinds, before: 0, limit: 500 })
                if (gone) return
                setList(p.events ?? [])
                setMore(p.more)
                setTotal(p.total)
                seenLive.current = p.events?.[0]?.id ?? live[live.length - 1]?.id ?? 0
            } finally {
                if (!gone) setLoading(false)
            }
        }, 180)
        return () => {
            gone = true
            clearTimeout(t)
        }
    }, [day, text, kinds, active]) // eslint-disable-line react-hooks/exhaustive-deps

    // Today's new events come in live.
    useEffect(() => {
        if (day !== today() || !live.length) return
        const fresh = live.filter(e => e.id > seenLive.current && eventMatches(e, text, kinds))
        if (live[live.length - 1].id > seenLive.current) seenLive.current = live[live.length - 1].id
        if (!fresh.length) return
        setList(old => [...fresh.reverse(), ...old])
        setTotal(n => n + fresh.length)
    }, [live]) // eslint-disable-line react-hooks/exhaustive-deps

    const loadMore = async () => {
        const last = list[list.length - 1]
        if (!last) return
        setLoading(true)
        try {
            const p = await api.searchLogs({ day, text, kinds, before: last.id, limit: 500 })
            setList(old => old.concat(p.events ?? []))
            setMore(p.more)
        } finally {
            setLoading(false)
        }
    }

    const exportCSV = async () => {
        try {
            const path = await api.exportLogs({ day, text, kinds, before: 0, limit: 0 })
            if (path) setMsg(`Saved to ${path}`)
        } catch (e) {
            setMsg(String(e))
        }
    }

    const v = useVirtualizer({ count: list.length + (more ? 1 : 0), getScrollElement: () => parent.current, estimateSize: () => 50, overscan: 10 })

    const dayOptions = days.some(d => d.day === today()) ? days : [{ day: today(), size: 0 }, ...days]

    return (
        <div className="page">
            <div className="page-head">
                <h1>Logs</h1>
                <select className="select" value={day} onChange={e => setDay(e.target.value)}>
                    {dayOptions.map(d => (
                        <option key={d.day} value={d.day}>
                            {d.day === today() ? "Today" : new Date(d.day + "T12:00").toLocaleDateString([], { weekday: "short", month: "short", day: "numeric" })}
                            {d.size ? ` · ${bytes(d.size)}` : ""}
                        </option>
                    ))}
                </select>
                <div className="seg">
                    {KINDS.map(k => (
                        <button key={k.id} className={kind === k.id ? "on" : ""} onClick={() => setKind(k.id)}>
                            {k.label}
                        </button>
                    ))}
                </div>
                <div className="grow" />
                <div className="search">
                    <Search size={15} />
                    <input className="input" placeholder="Search: program, host, country, port…" value={text} onChange={e => setText(e.target.value)} />
                </div>
                <button className="btn" onClick={exportCSV} title="Save these events as a CSV file">
                    <Download size={15} /> Export
                </button>
                <button className="btn icon" onClick={() => api.openLogsFolder()} title="Open the logs folder">
                    <FolderOpen size={15} />
                </button>
            </div>
            <div className="subtle" style={{ padding: "8px 18px", borderBottom: "1px solid var(--line)", fontSize: 12 }}>
                {loading ? "Searching…" : `${count(total)} event${total === 1 ? "" : "s"}`}
                {msg && <span className="dim"> · {msg}</span>}
            </div>
            <div className="scroll selectable" ref={parent}>
                {!list.length && !loading ? (
                    <div className="empty">{text || kinds.length ? "Nothing matches." : "Nothing logged on this day."}</div>
                ) : (
                    <div style={{ height: v.getTotalSize(), position: "relative" }}>
                        {v.getVirtualItems().map(item => (
                            <div key={item.key} data-index={item.index} ref={v.measureElement} style={{ position: "absolute", top: 0, left: 0, right: 0, transform: `translateY(${item.start}px)` }}>
                                {item.index < list.length ? (
                                    <EventRow e={list[item.index]} />
                                ) : (
                                    <div style={{ padding: 12, textAlign: "center" }}>
                                        <button className="btn small" onClick={loadMore} disabled={loading}>
                                            Show older
                                        </button>
                                    </div>
                                )}
                            </div>
                        ))}
                    </div>
                )}
            </div>
        </div>
    )
}
