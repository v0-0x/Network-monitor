import { ArrowDownLeft, ArrowUpRight, X, Radio, Info, AlertTriangle, CircleSlash } from "lucide-react"
import type { LogEvent } from "../lib/types"
import { clock, duration, eventText } from "../lib/format"

export function EventIcon({ e }: { e: LogEvent }) {
    const p = { size: 14, strokeWidth: 2 }
    switch (e.kind) {
        case "open":
            return e.dir === "in" ? <ArrowDownLeft {...p} className="k-open in" /> : <ArrowUpRight {...p} className="k-open" />
        case "close":
            return <X {...p} className="k-close" />
        case "listen":
            return <Radio {...p} className="k-listen" />
        case "unlisten":
            return <CircleSlash {...p} className="k-unlisten" />
        case "error":
            return <AlertTriangle {...p} className="k-error" />
        default:
            return <Info {...p} className="k-note" />
    }
}

export function eventMeta(e: LogEvent): string {
    const parts: string[] = []
    if (e.kind === "open" || e.kind === "close") {
        if (e.ip) parts.push(e.host ? `${e.ip}:${e.port}` : `port ${e.port}`)
        if (e.service) parts.push(e.service)
        if (e.org) parts.push(e.org)
        if (e.kind === "close" && e.dur) parts.push(`open ${duration(e.dur)}`)
        if (e.existing) parts.push("already open at start")
    } else if (e.kind === "listen" || e.kind === "unlisten") {
        if (e.local) parts.push(e.local)
        if (e.scope === "public") parts.push("reachable from other computers")
        else if (e.scope === "loopback") parts.push("this computer only")
    } else if (e.place) {
        parts.push(e.place)
    }
    return parts.join(" · ")
}

export function EventRow({ e }: { e: LogEvent }) {
    const meta = eventMeta(e)
    return (
        <div className="log-row">
            <span className="time">{clock(e.t)}</span>
            <EventIcon e={e} />
            <div className="text">
                <div>{eventText(e)}</div>
                {meta && <div className="meta">{meta}</div>}
            </div>
        </div>
    )
}
