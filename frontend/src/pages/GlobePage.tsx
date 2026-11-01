import { useMemo, useState } from "react"
import { Crosshair, MapPin, Minus, Plus } from "lucide-react"
import { GlobeView } from "../components/GlobeView"
import { HostList } from "../components/HostList"
import { EventRow } from "../components/EventRow"
import { groupHosts, useStore } from "../lib/store"
import { api } from "../lib/api"
import { bytes, bytesRate, count } from "../lib/format"
import type { Page } from "../components/Sidebar"

export function GlobePage({ active, goto }: { active: boolean; goto: (p: Page) => void }) {
    const { snap, events, geo, settings, focus, setFocus, selected, select } = useStore()
    const [tab, setTab] = useState<"hosts" | "log">("hosts")
    const [flyTo, setFlyTo] = useState<{ lat: number; lng: number; n: number } | null>(null)
    const groups = useMemo(() => groupHosts(snap, !!settings?.showLocal), [snap, settings?.showLocal])
    const publicGroups = useMemo(() => groups.filter(g => g.scope === "public"), [groups])
    const me = snap?.me ?? null
    const countries = useMemo(() => new Set(publicGroups.map(g => g.host?.countryCode).filter(Boolean)).size, [publicGroups])
    const publicConns = publicGroups.reduce((n, g) => n + g.conns.length, 0)
    const recent = useMemo(() => events.slice(-200).reverse(), [events])
    const located = publicGroups.filter(g => g.host?.hasCoords).length

    const pick = (ip: string | null) => {
        select(ip)
        const h = ip ? snap?.hosts?.[ip] : null
        if (h?.hasCoords) setFlyTo({ lat: h.lat, lng: h.lon, n: Date.now() })
    }
    const centerMe = () => me?.known && setFlyTo({ lat: me.lat - 18, lng: me.lon, n: Date.now() })

    return (
        <div className={"page" + (active ? "" : " hidden")}>
            <div className="page-head">
                <div className="stats">
                    <Stat v={count(publicConns)} l="Connections" />
                    <Stat v={count(publicGroups.length)} l="Hosts" />
                    <Stat v={count(countries)} l="Countries" />
                    <Stat v={bytesRate(snap?.rx ?? 0)} l="Download" />
                    <Stat v={bytesRate(snap?.tx ?? 0)} l="Upload" />
                </div>
                <div className="me-chip">
                    {snap?.demo && <span className="demo-tag">Demo data</span>}
                    <MapPin size={15} />
                    {me?.known ? (
                        <span className="ellipsis" title={me.ip ? `Your public IP: ${me.ip}` : undefined}>
                            {me.label || "Your location"}
                            {me.ip && <span className="subtle mono"> · {me.ip}</span>}
                        </span>
                    ) : (
                        <button className="link" onClick={() => goto("settings")}>
                            {me?.error && !me.waiting ? "Location unknown: set it" : "Finding your location…"}
                        </button>
                    )}
                </div>
            </div>
            <div className="globe-wrap">
                <div className="globe-area">
                    <GlobeView
                        groups={groups}
                        me={me}
                        active={active}
                        focus={focus}
                        selected={selected}
                        onHover={setFocus}
                        onSelect={pick}
                        flyTo={flyTo}
                    />
                    {geo && !geo.ready && (
                        <div className="globe-overlay notice">
                            <div className="title">{geo.downloading ? "Downloading the location database" : "No location database yet"}</div>
                            <div className="dim">
                                {geo.downloading
                                    ? `${geo.stage ?? "Database"}: ${bytes(geo.done)}${geo.total > 0 ? ` of ${bytes(geo.total)}` : ""}. It's downloaded once and works offline after that.`
                                    : geo.error || "The globe needs it to place addresses."}
                            </div>
                            {geo.downloading && geo.total > 0 && (
                                <div className="progress">
                                    <div style={{ width: `${Math.min(100, (geo.done / geo.total) * 100)}%` }} />
                                </div>
                            )}
                            {!geo.downloading && (
                                <div>
                                    <button className="btn small" onClick={() => api.updateGeo()}>
                                        Try again
                                    </button>
                                </div>
                            )}
                        </div>
                    )}
                    {geo?.ready && me && !me.known && me.error && !me.waiting && (
                        <div className="globe-overlay notice">
                            <div className="title">Where are you?</div>
                            <div className="dim">{me.error}</div>
                            <div>
                                <button className="btn small" onClick={() => goto("settings")}>
                                    Set your location
                                </button>
                            </div>
                        </div>
                    )}
                    <div className="globe-overlay legend">
                        <div className="item">
                            <span className="line" /> Outgoing: you connected
                        </div>
                        <div className="item">
                            <span className="line in" /> Incoming: they connected to you
                        </div>
                        {located < publicGroups.length && geo?.ready && <div className="subtle">{publicGroups.length - located} not on the map (no location known)</div>}
                        <button className="link credit" onClick={() => api.openURL("https://db-ip.com")}>
                            IP Geolocation by DB-IP
                        </button>
                    </div>
                    <div className="globe-overlay globe-tools">
                        <button className="btn icon" title="Center on you" onClick={centerMe} disabled={!me?.known}>
                            <Crosshair size={16} />
                        </button>
                        <ZoomButtons />
                    </div>
                </div>
                <div className="side-col">
                    <div className="tabs">
                        <div className="seg">
                            <button className={tab === "hosts" ? "on" : ""} onClick={() => setTab("hosts")}>
                                Connected now
                            </button>
                            <button className={tab === "log" ? "on" : ""} onClick={() => setTab("log")}>
                                Live log
                            </button>
                        </div>
                    </div>
                    <div className="scroll">
                        {tab === "hosts" ? (
                            <HostList groups={groups} me={me} focus={focus} selected={selected} onHover={setFocus} onSelect={pick} />
                        ) : recent.length ? (
                            recent.map(e => <EventRow key={e.id} e={e} />)
                        ) : (
                            <div className="empty">Nothing logged yet.</div>
                        )}
                    </div>
                </div>
            </div>
        </div>
    )
}

function Stat({ v, l }: { v: string; l: string }) {
    return (
        <div className="stat">
            <span className="v">{v}</span>
            <span className="l">{l}</span>
        </div>
    )
}

// Zoom with buttons too (not everyone has a scroll wheel).
function ZoomButtons() {
    const zoom = (f: number) => {
        const canvas = document.querySelector(".globe-canvas canvas")
        canvas?.dispatchEvent(new WheelEvent("wheel", { deltaY: f * 120, bubbles: true, cancelable: true }))
    }
    return (
        <>
            <button className="btn icon" title="Zoom out" onClick={() => zoom(1)}>
                <Minus size={16} />
            </button>
            <button className="btn icon" title="Zoom in" onClick={() => zoom(-1)}>
                <Plus size={16} />
            </button>
        </>
    )
}
