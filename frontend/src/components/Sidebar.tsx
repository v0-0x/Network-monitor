import type { ReactNode } from "react"
import { Globe2, List, ScrollText, Settings as SettingsIcon, ArrowDownToLine, RotateCw } from "lucide-react"
import { useStore } from "../lib/store"
import { api } from "../lib/api"
import { bytesRate, count } from "../lib/format"

export type Page = "globe" | "connections" | "logs" | "settings"

export function Sidebar({ page, setPage }: { page: Page; setPage: (p: Page) => void }) {
    const { snap, update, info } = useStore()
    const conns = snap?.conns?.filter(c => c.scope === "public").length ?? 0
    const ok = !!snap && !snap.error
    const items: { id: Page; label: string; icon: ReactNode; count?: number }[] = [
        { id: "globe", label: "Globe", icon: <Globe2 size={17} strokeWidth={1.75} /> },
        { id: "connections", label: "Connections", icon: <List size={17} strokeWidth={1.75} />, count: snap ? (snap.conns?.length ?? 0) : undefined },
        { id: "logs", label: "Logs", icon: <ScrollText size={17} strokeWidth={1.75} /> },
        { id: "settings", label: "Settings", icon: <SettingsIcon size={17} strokeWidth={1.75} /> },
    ]
    const av = update?.available
    return (
        <aside className="sidebar">
            {items.map(it => (
                <button key={it.id} className={"nav" + (page === it.id ? " active" : "")} onClick={() => setPage(it.id)} title={it.label}>
                    {it.icon}
                    <span>{it.label}</span>
                    {it.count !== undefined && <em className="count num">{count(it.count)}</em>}
                </button>
            ))}
            <div className="side-spacer" />
            <div className="side-status">
                <div className="row">
                    <i className={"pulse" + (ok ? "" : " off")} />
                    <span>{!snap ? "Starting…" : ok ? "Monitoring" : "Can't read connections"}</span>
                    <span className="num">{snap ? count(conns) : ""}</span>
                </div>
                <div className="row">
                    <span>Download</span>
                    <span className="num">{bytesRate(snap?.rx ?? 0)}</span>
                </div>
                <div className="row">
                    <span>Upload</span>
                    <span className="num">{bytesRate(snap?.tx ?? 0)}</span>
                </div>
            </div>
            {av && !update?.manual && (
                <div className="side-update">
                    <div className="text">
                        {update.ready ? (
                            <>Version {av.version} is installed.</>
                        ) : update.downloading || update.installing ? (
                            <>Installing {av.version}…</>
                        ) : (
                            <>
                                Version {av.version}
                                {av.beta ? " beta" : ""} is available.
                            </>
                        )}
                    </div>
                    {update.ready ? (
                        <button className="btn small primary" onClick={() => api.restartToUpdate()} title="Restart to update">
                            <RotateCw size={14} />
                            <span className="text">Restart</span>
                        </button>
                    ) : (
                        <button className="btn small" disabled={update.downloading || update.installing} onClick={() => api.installUpdate()} title="Install the update">
                            <ArrowDownToLine size={14} />
                            <span className="text">{update.downloading || update.installing ? "Installing…" : "Install"}</span>
                        </button>
                    )}
                </div>
            )}
            <div className="version">
                {info ? `Version ${info.version}${info.channel === "beta" ? " beta" : ""}` : ""}
            </div>
        </aside>
    )
}
