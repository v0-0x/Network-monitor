import { useEffect, useRef } from "react"
import Globe, { type GlobeInstance } from "globe.gl"
import { feature } from "topojson-client"
import countriesTopo from "world-atlas/countries-110m.json"
import type { HostGroup } from "../lib/store"
import { latency } from "../lib/store"
import type { Me } from "../lib/types"
import { bytes, bytesRate, coords, count, duration, esc, ms, place, riskName } from "../lib/format"

// The globe: one line per remote address, from it to you (or from you to
// it), with a light running along it in the direction the connection was
// opened. Hovering a line or a dot shows who it is.

type ArcD = {
    kind: "arc" | "pulse"
    ip: string
    startLat: number
    startLng: number
    endLat: number
    endLng: number
    alt: number
    dist: number // radians
    seed: number
    g: HostGroup
}

type PointD = {
    kind: "point" | "me"
    ip: string
    lat: number
    lng: number
    g?: HostGroup
}

type Entry = { arc: ArcD; pulse: ArcD; point: PointD; dir: string }

const OUT = "226, 226, 230"
const IN = "138, 180, 248"
const MEDIUM = "232, 176, 75"
const HIGH = "239, 107, 107"

// A line's colour: its risk when that's medium or high, else its direction.
function rgbOf(g: HostGroup): string {
    const lvl = g.host?.risk?.level
    if (lvl === "high") return HIGH
    if (lvl === "medium") return MEDIUM
    return g.dir === "in" ? IN : OUT
}

function hash(s: string): number {
    let h = 2166136261
    for (let i = 0; i < s.length; i++) h = Math.imul(h ^ s.charCodeAt(i), 16777619)
    return ((h >>> 0) % 10000) / 10000
}

const rad = (d: number) => (d * Math.PI) / 180

// Great-circle distance in radians.
function angle(lat1: number, lon1: number, lat2: number, lon2: number): number {
    const a = Math.sin(rad(lat2 - lat1) / 2) ** 2 + Math.cos(rad(lat1)) * Math.cos(rad(lat2)) * Math.sin(rad(lon2 - lon1) / 2) ** 2
    return 2 * Math.atan2(Math.sqrt(a), Math.sqrt(1 - a))
}

export function distanceKm(lat1: number, lon1: number, lat2: number, lon2: number): number {
    return angle(lat1, lon1, lat2, lon2) * 6371
}

function tipHtml(g: HostGroup, me: Me | null): string {
    const h = g.host
    const procs = new Map<string, number[]>()
    for (const c of g.conns) {
        const k = c.process || "Unknown program"
        if (!procs.has(k)) procs.set(k, [])
        if (c.pid) procs.get(k)!.push(c.pid)
    }
    const procText = [...procs.entries()]
        .slice(0, 4)
        .map(([n, pids]) => `${esc(n)}${pids.length ? ` <span class="subtle">PID ${esc([...new Set(pids)].slice(0, 2).join(", "))}</span>` : ""}`)
        .join("<br>")
    const ports = new Map<string, number>()
    for (const c of g.conns) {
        const port = c.dir === "in" ? c.localPort : c.remotePort
        const k = `${port}${c.service ? " " + c.service : ""} · ${c.proto.toUpperCase()}`
        ports.set(k, (ports.get(k) || 0) + 1)
    }
    const portText = [...ports.entries()]
        .slice(0, 3)
        .map(([k, n]) => esc(k) + (n > 1 ? ` <span class="subtle">×${n}</span>` : ""))
        .join("<br>")
    const dirText = g.dir === "in" ? "Incoming: they connected to you" : "Outgoing: you connected to them"
    const rows: [string, string][] = [
        ["Program", procText],
        ["Direction", `<span class="dir"><span class="dot ${g.dir === "in" ? "in" : ""}"></span>${dirText}</span>`],
        [g.dir === "in" ? "Your port" : "Port", portText],
    ]
    const where = place(h)
    if (where) rows.push(["Location", esc(where) + (h?.postcode ? ` <span class="subtle">${esc(h.postcode)}</span>` : "")])
    if (h?.hasCoords) rows.push(["Coordinates", esc(coords(h.lat, h.lon))])
    if (h?.org || h?.asn) rows.push(["Network", esc(h?.org || "") + (h?.asn ? ` <span class="subtle">AS${h.asn}</span>` : "")])
    if (h?.timeZone) rows.push(["Time zone", esc(h.timeZone)])
    if (h?.hasCoords && me?.known) rows.push(["Distance", `${count(Math.round(distanceKm(me.lat, me.lon, h.lat, h.lon)))} km from you`])
    rows.push(["Open for", esc(duration(Date.now() - g.since)) + (g.conns.length > 1 ? ` <span class="subtle">· ${g.conns.length} connections</span>` : "")])
    if (g.hasBytes) {
        const now = g.rxRate + g.txRate > 0 ? `<br><span class="subtle">now ↓ ${esc(bytesRate(g.rxRate))} ↑ ${esc(bytesRate(g.txRate))}</span>` : ""
        rows.push(["Traffic", `↓ ${esc(bytes(g.rx))} ↑ ${esc(bytes(g.tx))}${now}`])
    }
    const lat = latency(g)
    if (lat !== undefined) {
        const p = h?.ping
        const how = p && p.received > 0 ? (p.method === "icmp" ? "ping" : "TCP") + (p.sent > p.received ? `, ${Math.round(((p.sent - p.received) / p.sent) * 100)}% lost` : "") : "connection"
        rows.push(["Latency", `${esc(ms(lat))} <span class="subtle">${how}</span>`])
    } else if (h?.ping && h.ping.received === 0) {
        rows.push(["Latency", `<span class="subtle">no answer to ping</span>`])
    }
    const r = h?.risk
    if (r) {
        const why = r.reasons.length ? `<ul class="why">${r.reasons.slice(0, 4).map(x => `<li>${esc(x)}</li>`).join("")}</ul>` : ""
        rows.push(["Risk", `<span class="risk ${r.level}">${riskName[r.level]} · ${r.score}</span>${why}`])
    }
    const who = h?.name || h?.org || g.ip
    return `<div class="tip"><div class="head"><div class="who">${esc(who)}</div>${who !== g.ip ? `<div class="ip">${esc(g.ip)}</div>` : ""}</div><dl class="kv">${rows.map(([k, v]) => `<dt>${k}</dt><dd>${v}</dd>`).join("")}</dl></div>`
}

function meHtml(me: Me): string {
    const rows: [string, string][] = []
    if (me.label) rows.push(["Location", esc(me.label)])
    rows.push(["Coordinates", esc(coords(me.lat, me.lon))])
    if (me.ip) rows.push(["Public IP", `<span class="mono">${esc(me.ip)}</span>`])
    if (me.org) rows.push(["Provider", esc(me.org)])
    rows.push(["Set", me.manual ? "By hand, in Settings" : "From your public IP address"])
    return `<div class="tip"><div class="head"><div class="who">You</div></div><dl class="kv">${rows.map(([k, v]) => `<dt>${k}</dt><dd>${v}</dd>`).join("")}</dl></div>`
}

interface Props {
    groups: HostGroup[]
    me: Me | null
    active: boolean
    focus: string | null
    selected: string | null
    onHover: (ip: string | null) => void
    onSelect: (ip: string | null) => void
    flyTo: { lat: number; lng: number; n: number } | null
}

export function GlobeView({ groups, me, active, focus, selected, onHover, onSelect, flyTo }: Props) {
    const el = useRef<HTMLDivElement>(null)
    const globe = useRef<GlobeInstance | null>(null)
    const entries = useRef(new Map<string, Entry>())
    const meRef = useRef<Me | null>(null)
    const hl = useRef<{ focus: string | null; selected: string | null }>({ focus: null, selected: null })
    const centered = useRef(false)
    const styleSig = useRef("")
    const cb = useRef({ onHover, onSelect })
    cb.current = { onHover, onSelect }

    // Create the globe once.
    useEffect(() => {
        const container = el.current!
        const topo = countriesTopo as any // eslint-disable-line @typescript-eslint/no-explicit-any
        const countries = (feature(topo, topo.objects.countries) as any).features // eslint-disable-line @typescript-eslint/no-explicit-any
        const g = new Globe(container, { animateIn: false, rendererConfig: { antialias: true, alpha: true, powerPreference: "high-performance" } })
            .backgroundColor("rgba(0,0,0,0)")
            .showAtmosphere(true)
            .atmosphereColor("#8f97a8")
            .atmosphereAltitude(0.13)
            .polygonsData(countries)
            .polygonCapColor(() => "#212126")
            .polygonSideColor(() => "rgba(0,0,0,0)")
            .polygonStrokeColor(() => "#3b3b42")
            .polygonAltitude(0.004)
            .polygonsTransitionDuration(0)
            .arcStartLat("startLat")
            .arcStartLng("startLng")
            .arcEndLat("endLat")
            .arcEndLng("endLng")
            .arcAltitude("alt")
            .arcCurveResolution(48)
            .arcCircularResolution(4)
            .arcsTransitionDuration(700)
            .arcDashLength((d: object) => ((d as ArcD).kind === "pulse" ? 0.07 : 1))
            .arcDashGap((d: object) => ((d as ArcD).kind === "pulse" ? 0.93 : 0))
            .arcDashInitialGap((d: object) => ((d as ArcD).kind === "pulse" ? (d as ArcD).seed : 0))
            .arcDashAnimateTime((d: object) => ((d as ArcD).kind === "pulse" ? 1300 + (d as ArcD).dist * 1100 : 0))
            .arcLabel((d: object) => ((d as ArcD).kind === "arc" ? tipHtml((d as ArcD).g, meRef.current) : ""))
            .pointLat("lat")
            .pointLng("lng")
            .pointAltitude((d: object) => ((d as PointD).kind === "me" ? 0.012 : 0.006))
            .pointRadius((d: object) => ((d as PointD).kind === "me" ? 0.42 : 0.2 + Math.min(0.25, ((d as PointD).g?.conns.length || 1) * 0.02)))
            .pointResolution(12)
            .pointsMerge(false)
            .pointsTransitionDuration(0)
            .pointLabel((d: object) => {
                const p = d as PointD
                if (p.kind === "me") return meRef.current ? meHtml(meRef.current) : ""
                return p.g ? tipHtml(p.g, meRef.current) : ""
            })
            .ringLat("lat")
            .ringLng("lng")
            .ringColor(() => (t: number) => `rgba(233, 230, 223, ${0.55 * (1 - t)})`)
            .ringMaxRadius(3.2)
            .ringPropagationSpeed(1.6)
            .ringRepeatPeriod(1600)
            .lineHoverPrecision(1.2)
            .pointerEventsFilter((_o: unknown, data?: object) => {
                const k = (data as { kind?: string } | undefined)?.kind
                return k === "arc" || k === "point" || k === "me"
            })
            .showPointerCursor((type: string, d: object) => !!d && (type === "arc" || type === "point"))
            .onArcHover((d: object | null) => cb.current.onHover(d ? (d as ArcD).ip : null))
            .onPointHover((d: object | null) => cb.current.onHover(d && (d as PointD).kind === "point" ? (d as PointD).ip : null))
            .onArcClick((d: object) => cb.current.onSelect((d as ArcD).ip))
            .onPointClick((d: object) => {
                const p = d as PointD
                cb.current.onSelect(p.kind === "point" ? p.ip : null)
            })
            .onGlobeClick(() => cb.current.onSelect(null))
        // A dark, matte sphere.
        const mat = g.globeMaterial() as unknown as { color: { set: (c: string) => void }; shininess: number; emissive?: { set: (c: string) => void } }
        mat.color.set("#141418")
        mat.emissive?.set("#08080a")
        mat.shininess = 3
        const controls = g.controls() as unknown as { minDistance: number; maxDistance: number; enableDamping: boolean; dampingFactor: number; rotateSpeed: number; zoomSpeed: number }
        controls.minDistance = 140
        controls.maxDistance = 650
        controls.enableDamping = true
        controls.dampingFactor = 0.12
        controls.rotateSpeed = 0.6
        controls.zoomSpeed = 0.7
        g.pointOfView({ lat: 25, lng: 10, altitude: 2.4 }, 0)
        globe.current = g
        if (import.meta.env.DEV) (window as unknown as { __globe: GlobeInstance }).__globe = g

        const ro = new ResizeObserver(() => {
            g.width(container.clientWidth).height(container.clientHeight)
        })
        ro.observe(container)
        return () => {
            ro.disconnect()
            g.pauseAnimation()
            g._destructor()
            container.innerHTML = ""
            globe.current = null
        }
    }, [])

    // Only drawn while the page shows and the window is visible.
    useEffect(() => {
        const g = globe.current
        if (!g) return
        const apply = () => {
            if (active && !document.hidden) g.resumeAnimation()
            else g.pauseAnimation()
        }
        apply()
        document.addEventListener("visibilitychange", apply)
        return () => document.removeEventListener("visibilitychange", apply)
    }, [active])

    // Keep the lines in step with the connections.
    useEffect(() => {
        const g = globe.current
        if (!g) return
        const meMoved = !meRef.current || !me || meRef.current.lat !== me.lat || meRef.current.lon !== me.lon || meRef.current.known !== me.known
        meRef.current = me
        const map = entries.current
        let changed = meMoved
        if (meMoved) map.clear()
        const seen = new Set<string>()
        if (me?.known) {
            for (const grp of groups) {
                const h = grp.host
                if (grp.scope !== "public" || !h?.hasCoords) continue
                seen.add(grp.ip)
                const e = map.get(grp.ip)
                if (e && e.dir === grp.dir) {
                    e.arc.g = e.pulse.g = e.point.g = grp
                    continue
                }
                const out = grp.dir !== "in"
                const [sLat, sLng, eLat, eLng] = out ? [me.lat, me.lon, h.lat, h.lon] : [h.lat, h.lon, me.lat, me.lon]
                const dist = angle(sLat, sLng, eLat, eLng)
                const seed = hash(grp.ip)
                // Lines to the same city fan out a little, so each can be hovered.
                const alt = (0.03 + dist * 0.11) * (0.82 + seed * 0.36)
                const base = { ip: grp.ip, startLat: sLat, startLng: sLng, endLat: eLat, endLng: eLng, alt, dist, seed, g: grp }
                map.set(grp.ip, {
                    arc: { kind: "arc", ...base },
                    pulse: { kind: "pulse", ...base },
                    point: { kind: "point", ip: grp.ip, lat: h.lat, lng: h.lon, g: grp },
                    dir: grp.dir,
                })
                changed = true
            }
        }
        for (const ip of [...map.keys()]) {
            if (!seen.has(ip)) {
                map.delete(ip)
                changed = true
            }
        }
        // Colours and widths follow risk and traffic, which change between scans.
        const sig = groups.map(g => `${g.ip}:${g.host?.risk?.level}:${Math.round(Math.log10(1 + (g.rxRate + g.txRate) / 1000) * 3)}`).join(",")
        if (sig !== styleSig.current) {
            styleSig.current = sig
            restyle()
        }
        if (!changed) return
        const list = [...map.values()]
        g.arcsData([...list.map(e => e.arc), ...list.map(e => e.pulse)])
        const points: PointD[] = list.map(e => e.point)
        if (me?.known) points.push({ kind: "me", ip: "", lat: me.lat, lng: me.lon })
        g.pointsData(points)
        g.ringsData(me?.known ? [{ lat: me.lat, lng: me.lon }] : [])
        if (me?.known && !centered.current) {
            centered.current = true
            g.pointOfView({ lat: me.lat - 18, lng: me.lon, altitude: 2.3 }, 1200)
        }
        restyle()
    }, [groups, me]) // eslint-disable-line react-hooks/exhaustive-deps

    // Hovered and picked lines stand out.
    const restyle = () => {
        const g = globe.current
        if (!g) return
        const { focus, selected } = hl.current
        const any = focus || selected
        const on = (ip: string) => ip === focus || ip === selected
        g.arcColor((d: object) => {
            const a = d as ArcD
            const rgb = rgbOf(a.g)
            const plain = rgb === OUT
            if (a.kind === "pulse") return on(a.ip) ? `rgba(255,255,255,1)` : `rgba(${rgb}, ${any ? 0.35 : 0.95})`
            if (on(a.ip)) return plain ? "rgba(255,255,255,0.95)" : `rgba(${rgb}, 1)`
            return `rgba(${rgb}, ${any ? 0.12 : plain ? 0.32 : 0.55})`
        })
        g.arcStroke((d: object) => {
            const a = d as ArcD
            // Busier connections draw thicker lines.
            const busy = Math.min(0.3, Math.log10(1 + (a.g.rxRate + a.g.txRate) / 1000) * 0.1)
            if (a.kind === "pulse") return (on(a.ip) ? 0.75 : 0.5) + busy
            return (on(a.ip) ? 0.55 : 0.26) + busy
        })
        g.pointColor((d: object) => {
            const p = d as PointD
            if (p.kind === "me") return "#f4f2ec"
            const rgb = p.g ? rgbOf(p.g) : OUT
            return on(p.ip) ? "#ffffff" : `rgba(${rgb}, ${any ? 0.4 : 0.85})`
        })
    }

    useEffect(() => {
        hl.current = { focus, selected }
        restyle()
    }, [focus, selected]) // eslint-disable-line react-hooks/exhaustive-deps

    useEffect(() => {
        if (flyTo && globe.current) globe.current.pointOfView({ lat: flyTo.lat, lng: flyTo.lng, altitude: 1.9 }, 900)
    }, [flyTo])

    return <div ref={el} className="globe-canvas" />
}
