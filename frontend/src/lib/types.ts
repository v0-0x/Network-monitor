// The Go side's data, as the page sees it (internal/monitor, internal/logs,
// internal/geo, internal/update, internal/settings).

export interface Location {
    city?: string
    region?: string
    country?: string
    countryCode?: string
    lat: number
    lon: number
    hasCoords: boolean
    timeZone?: string
    postcode?: string
    asn?: number
    org?: string
}

export interface Conn {
    id: string
    proto: "tcp" | "udp"
    localIp: string
    localPort: number
    remoteIp: string
    remotePort: number
    state: string
    pid: number
    process: string
    exe?: string
    dir: "in" | "out"
    scope: "public" | "local" | "loopback"
    service?: string
    since: number
    existing?: boolean
}

export interface Host extends Location {
    ip: string
    name?: string
    scope: "public" | "local" | "loopback"
}

export interface Listener {
    proto: "tcp" | "udp"
    ip: string
    port: number
    pid: number
    process: string
    exe?: string
    service?: string
    scope: string
    since: number
}

export interface Me extends Location {
    known: boolean
    manual: boolean
    ip?: string
    label: string
    error?: string
    waiting?: boolean
}

export interface Totals {
    since: number
    opened: number
    hosts: number
    countries: number
    inbound: number
}

export interface Snapshot {
    time: number
    conns: Conn[] | null
    hosts: Record<string, Host> | null
    listeners: Listener[] | null
    me: Me
    rx: number
    tx: number
    scanMs: number
    method: string
    error?: string
    demo: boolean
    totals: Totals
}

export type EventKind = "open" | "close" | "listen" | "unlisten" | "note" | "error"

export interface LogEvent {
    id: number
    t: number
    kind: EventKind
    proto?: string
    process?: string
    pid?: number
    local?: string
    ip?: string
    port?: number
    service?: string
    host?: string
    place?: string
    cc?: string
    org?: string
    dir?: "in" | "out"
    scope?: string
    dur?: number
    existing?: boolean
    msg?: string
}

export interface GeoStatus {
    ready: boolean
    hasAsn: boolean
    downloading: boolean
    stage?: string
    done: number
    total: number
    error?: string
    cityVersion?: string
    asnVersion?: string
    from?: string
    downloaded: string
    sizeBytes: number
}

export interface UpdateState {
    current: string
    channel: string
    beta: boolean
    checking: boolean
    lastCheck: string
    error?: string
    available?: { version: string; beta: boolean; notes: string; url: string; size: number }
    downloading: boolean
    done: number
    total: number
    installing: boolean
    ready: boolean
    manual?: string
}

export interface Tick {
    snapshot: Snapshot
    events: LogEvent[] | null
    geo: GeoStatus
    update: UpdateState
}

export interface Info {
    version: string
    channel: string
    commit: string
    os: string
    arch: string
    dataDir: string
    logsDir: string
    demo: boolean
}

export interface Settings {
    location: { mode: "auto" | "manual"; lat: number; lon: number; label: string }
    intervalMs: number
    resolveNames: boolean
    showLocal: boolean
    logDays: number
    geoAuto: boolean
    updates: { auto: boolean; beta: boolean; betaChosen: boolean; skip: string }
}

export interface LogQuery {
    day: string
    text: string
    kinds: string[]
    before: number
    limit: number
}

export interface LogPage {
    events: LogEvent[]
    more: boolean
    total: number
}

export interface LogDay {
    day: string
    size: number
}
