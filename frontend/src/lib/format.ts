import type { Conn, Host, LogEvent } from "./types"

export function bytesRate(n: number): string {
    if (!isFinite(n) || n < 0) n = 0
    if (n < 1000) return `${Math.round(n)} B/s`
    if (n < 1e6) return `${(n / 1e3).toFixed(n < 1e4 ? 1 : 0)} KB/s`
    if (n < 1e9) return `${(n / 1e6).toFixed(n < 1e7 ? 1 : 0)} MB/s`
    return `${(n / 1e9).toFixed(1)} GB/s`
}

export function bytes(n: number): string {
    if (n < 1024) return `${n} B`
    if (n < 1024 ** 2) return `${(n / 1024).toFixed(0)} KB`
    if (n < 1024 ** 3) return `${(n / 1024 ** 2).toFixed(n < 10 * 1024 ** 2 ? 1 : 0)} MB`
    return `${(n / 1024 ** 3).toFixed(1)} GB`
}

export function duration(ms: number): string {
    const s = Math.max(0, Math.floor(ms / 1000))
    if (s < 60) return `${s}s`
    const m = Math.floor(s / 60)
    if (m < 60) return `${m}m ${s % 60}s`
    const h = Math.floor(m / 60)
    if (h < 24) return `${h}h ${m % 60}m`
    return `${Math.floor(h / 24)}d ${h % 24}h`
}

export function clock(ms: number): string {
    return new Date(ms).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit", hour12: false })
}

// "2.3.2026060513" (the npm package) or "2026-11" (DB-IP's own file) -> "2026-11".
export function dbVersion(v?: string): string {
    if (!v) return ""
    const m = /(\d{4})(\d{2})\d*$/.exec(v)
    if (v.includes(".") && m) return `${m[1]}-${m[2]}`
    return v
}

export function count(n: number): string {
    return n.toLocaleString()
}

export function coords(lat: number, lon: number): string {
    const ns = lat >= 0 ? "N" : "S"
    const ew = lon >= 0 ? "E" : "W"
    return `${Math.abs(lat).toFixed(2)}°${ns}, ${Math.abs(lon).toFixed(2)}°${ew}`
}

export function place(h?: Partial<Host> | null): string {
    if (!h) return ""
    const parts: string[] = []
    if (h.city) parts.push(h.city)
    if (h.region && h.region !== h.city) parts.push(h.region)
    if (h.country) parts.push(h.country)
    return parts.join(", ")
}

export function shortPlace(h?: Partial<Host> | null): string {
    if (!h) return ""
    if (h.city && h.countryCode) return `${h.city}, ${h.countryCode}`
    return h.country || h.city || ""
}

export function hostLabel(ip: string, h?: Host): string {
    return h?.name || ip
}

export function endpoint(ip: string, port: number): string {
    return ip.includes(":") ? `[${ip}]:${port}` : `${ip}:${port}`
}

export function scopeLabel(scope: string): string {
    if (scope === "loopback") return "This computer"
    if (scope === "local") return "Local network"
    return "Internet"
}

export function stateLabel(c: Conn): string {
    if (c.proto === "udp") return "Connected"
    switch (c.state) {
        case "ESTABLISHED":
            return "Open"
        case "SYN_SENT":
            return "Connecting"
        case "SYN_RECV":
            return "Accepting"
        case "CLOSE_WAIT":
        case "FIN_WAIT1":
        case "FIN_WAIT2":
        case "LAST_ACK":
        case "CLOSING":
            return "Closing"
    }
    return c.state.toLowerCase()
}

export function eventText(e: LogEvent): string {
    const who = e.process || "Unknown program"
    const where = e.host || e.ip || ""
    const at = e.place ? ` (${e.place})` : ""
    switch (e.kind) {
        case "open":
            return e.dir === "in" ? `${where}${at} connected to ${who}` : `${who} connected to ${where}${at}`
        case "close":
            return e.dir === "in" ? `${where} disconnected from ${who}` : `${who} closed ${where}`
        case "listen":
            return `${who} opened port ${e.port}${e.proto ? "/" + e.proto.toUpperCase() : ""}${e.service ? ` (${e.service})` : ""}`
        case "unlisten":
            return `${who} closed port ${e.port}${e.proto ? "/" + e.proto.toUpperCase() : ""}`
        default:
            return e.msg || ""
    }
}

// esc makes text safe inside HTML: host names come from DNS and anyone can
// put anything in their reverse DNS.
export function esc(s: unknown): string {
    return String(s ?? "").replace(/[&<>"']/g, c => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]!)
}

export function eventMatches(e: LogEvent, text: string, kinds: string[]): boolean {
    if (kinds.length && !kinds.includes(e.kind)) return false
    const words = text.toLowerCase().split(/\s+/).filter(Boolean)
    if (!words.length) return true
    const dir = e.dir === "in" ? "incoming" : e.dir === "out" ? "outgoing" : ""
    const hay = [e.kind, dir, e.proto, e.process, e.local, e.ip, String(e.port ?? ""), e.service, e.host, e.place, e.cc, e.org, e.msg].join(" ").toLowerCase()
    return words.every(w => hay.includes(w))
}
