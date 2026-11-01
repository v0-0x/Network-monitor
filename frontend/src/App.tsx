import { useEffect, useState } from "react"
import { Sidebar, type Page } from "./components/Sidebar"
import { GlobePage } from "./pages/GlobePage"
import { ConnectionsPage } from "./pages/ConnectionsPage"
import { LogsPage } from "./pages/LogsPage"
import { SettingsPage } from "./pages/SettingsPage"
import { useStore } from "./lib/store"

export function App() {
    const [page, setPage] = useState<Page>("globe")
    const { info } = useStore()
    const mac = info?.os === "darwin"
    useEffect(() => {
        document.documentElement.classList.toggle("mac", mac)
    }, [mac])
    return (
        <div className={"shell" + (mac ? " mac" : "")}>
            {mac && <div className="drag" />}
            <Sidebar page={page} setPage={setPage} />
            <main className="panel">
                {/* The globe stays (its WebGL view is costly to make again); it only pauses. */}
                <GlobePage active={page === "globe"} goto={setPage} />
                {page === "connections" && <ConnectionsPage />}
                {page === "logs" && <LogsPage active />}
                {page === "settings" && <SettingsPage />}
            </main>
        </div>
    )
}
