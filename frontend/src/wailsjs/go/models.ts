export namespace geo {
	
	export class Status {
	    ready: boolean;
	    hasAsn: boolean;
	    downloading: boolean;
	    stage?: string;
	    done: number;
	    total: number;
	    error?: string;
	    cityVersion?: string;
	    asnVersion?: string;
	    from?: string;
	    // Go type: time
	    downloaded: any;
	    sizeBytes: number;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ready = source["ready"];
	        this.hasAsn = source["hasAsn"];
	        this.downloading = source["downloading"];
	        this.stage = source["stage"];
	        this.done = source["done"];
	        this.total = source["total"];
	        this.error = source["error"];
	        this.cityVersion = source["cityVersion"];
	        this.asnVersion = source["asnVersion"];
	        this.from = source["from"];
	        this.downloaded = this.convertValues(source["downloaded"], null);
	        this.sizeBytes = source["sizeBytes"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace logs {
	
	export class Day {
	    day: string;
	    size: number;
	
	    static createFrom(source: any = {}) {
	        return new Day(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.day = source["day"];
	        this.size = source["size"];
	    }
	}
	export class Event {
	    id: number;
	    t: number;
	    kind: string;
	    proto?: string;
	    process?: string;
	    pid?: number;
	    local?: string;
	    ip?: string;
	    port?: number;
	    service?: string;
	    host?: string;
	    place?: string;
	    cc?: string;
	    org?: string;
	    dir?: string;
	    scope?: string;
	    dur?: number;
	    existing?: boolean;
	    rx?: number;
	    tx?: number;
	    risk?: number;
	    msg?: string;
	
	    static createFrom(source: any = {}) {
	        return new Event(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.t = source["t"];
	        this.kind = source["kind"];
	        this.proto = source["proto"];
	        this.process = source["process"];
	        this.pid = source["pid"];
	        this.local = source["local"];
	        this.ip = source["ip"];
	        this.port = source["port"];
	        this.service = source["service"];
	        this.host = source["host"];
	        this.place = source["place"];
	        this.cc = source["cc"];
	        this.org = source["org"];
	        this.dir = source["dir"];
	        this.scope = source["scope"];
	        this.dur = source["dur"];
	        this.existing = source["existing"];
	        this.rx = source["rx"];
	        this.tx = source["tx"];
	        this.risk = source["risk"];
	        this.msg = source["msg"];
	    }
	}
	export class Page {
	    events: Event[];
	    more: boolean;
	    total: number;
	
	    static createFrom(source: any = {}) {
	        return new Page(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.events = this.convertValues(source["events"], Event);
	        this.more = source["more"];
	        this.total = source["total"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Query {
	    day: string;
	    text: string;
	    kinds: string[];
	    before: number;
	    limit: number;
	
	    static createFrom(source: any = {}) {
	        return new Query(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.day = source["day"];
	        this.text = source["text"];
	        this.kinds = source["kinds"];
	        this.before = source["before"];
	        this.limit = source["limit"];
	    }
	}

}

export namespace main {
	
	export class Info {
	    version: string;
	    channel: string;
	    commit: string;
	    os: string;
	    arch: string;
	    dataDir: string;
	    logsDir: string;
	    demo: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Info(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.channel = source["channel"];
	        this.commit = source["commit"];
	        this.os = source["os"];
	        this.arch = source["arch"];
	        this.dataDir = source["dataDir"];
	        this.logsDir = source["logsDir"];
	        this.demo = source["demo"];
	    }
	}
	export class Tick {
	    snapshot: monitor.Snapshot;
	    events: logs.Event[];
	    geo: geo.Status;
	    update: update.State;
	    blocklists: risk.Status;
	
	    static createFrom(source: any = {}) {
	        return new Tick(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.snapshot = this.convertValues(source["snapshot"], monitor.Snapshot);
	        this.events = this.convertValues(source["events"], logs.Event);
	        this.geo = this.convertValues(source["geo"], geo.Status);
	        this.update = this.convertValues(source["update"], update.State);
	        this.blocklists = this.convertValues(source["blocklists"], risk.Status);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace monitor {
	
	export class Conn {
	    id: string;
	    proto: string;
	    localIp: string;
	    localPort: number;
	    remoteIp: string;
	    remotePort: number;
	    state: string;
	    pid: number;
	    process: string;
	    exe?: string;
	    dir: string;
	    scope: string;
	    service?: string;
	    since: number;
	    existing?: boolean;
	    hasBytes?: boolean;
	    rx: number;
	    tx: number;
	    rxRate: number;
	    txRate: number;
	    rtt?: number;
	
	    static createFrom(source: any = {}) {
	        return new Conn(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.proto = source["proto"];
	        this.localIp = source["localIp"];
	        this.localPort = source["localPort"];
	        this.remoteIp = source["remoteIp"];
	        this.remotePort = source["remotePort"];
	        this.state = source["state"];
	        this.pid = source["pid"];
	        this.process = source["process"];
	        this.exe = source["exe"];
	        this.dir = source["dir"];
	        this.scope = source["scope"];
	        this.service = source["service"];
	        this.since = source["since"];
	        this.existing = source["existing"];
	        this.hasBytes = source["hasBytes"];
	        this.rx = source["rx"];
	        this.tx = source["tx"];
	        this.rxRate = source["rxRate"];
	        this.txRate = source["txRate"];
	        this.rtt = source["rtt"];
	    }
	}
	export class Host {
	    ip: string;
	    name?: string;
	    scope: string;
	    city?: string;
	    region?: string;
	    country?: string;
	    countryCode?: string;
	    lat: number;
	    lon: number;
	    hasCoords: boolean;
	    timeZone?: string;
	    postcode?: string;
	    asn?: number;
	    org?: string;
	    ping?: ping.Result;
	    risk: risk.Assessment;
	    listed?: string[];
	
	    static createFrom(source: any = {}) {
	        return new Host(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ip = source["ip"];
	        this.name = source["name"];
	        this.scope = source["scope"];
	        this.city = source["city"];
	        this.region = source["region"];
	        this.country = source["country"];
	        this.countryCode = source["countryCode"];
	        this.lat = source["lat"];
	        this.lon = source["lon"];
	        this.hasCoords = source["hasCoords"];
	        this.timeZone = source["timeZone"];
	        this.postcode = source["postcode"];
	        this.asn = source["asn"];
	        this.org = source["org"];
	        this.ping = this.convertValues(source["ping"], ping.Result);
	        this.risk = this.convertValues(source["risk"], risk.Assessment);
	        this.listed = source["listed"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Listener {
	    proto: string;
	    ip: string;
	    port: number;
	    pid: number;
	    process: string;
	    exe?: string;
	    service?: string;
	    scope: string;
	    since: number;
	
	    static createFrom(source: any = {}) {
	        return new Listener(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.proto = source["proto"];
	        this.ip = source["ip"];
	        this.port = source["port"];
	        this.pid = source["pid"];
	        this.process = source["process"];
	        this.exe = source["exe"];
	        this.service = source["service"];
	        this.scope = source["scope"];
	        this.since = source["since"];
	    }
	}
	export class Me {
	    known: boolean;
	    manual: boolean;
	    ip?: string;
	    label: string;
	    city?: string;
	    region?: string;
	    country?: string;
	    countryCode?: string;
	    lat: number;
	    lon: number;
	    hasCoords: boolean;
	    timeZone?: string;
	    postcode?: string;
	    asn?: number;
	    org?: string;
	    error?: string;
	    waiting?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Me(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.known = source["known"];
	        this.manual = source["manual"];
	        this.ip = source["ip"];
	        this.label = source["label"];
	        this.city = source["city"];
	        this.region = source["region"];
	        this.country = source["country"];
	        this.countryCode = source["countryCode"];
	        this.lat = source["lat"];
	        this.lon = source["lon"];
	        this.hasCoords = source["hasCoords"];
	        this.timeZone = source["timeZone"];
	        this.postcode = source["postcode"];
	        this.asn = source["asn"];
	        this.org = source["org"];
	        this.error = source["error"];
	        this.waiting = source["waiting"];
	    }
	}
	export class Totals {
	    since: number;
	    opened: number;
	    hosts: number;
	    countries: number;
	    inbound: number;
	
	    static createFrom(source: any = {}) {
	        return new Totals(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.since = source["since"];
	        this.opened = source["opened"];
	        this.hosts = source["hosts"];
	        this.countries = source["countries"];
	        this.inbound = source["inbound"];
	    }
	}
	export class Snapshot {
	    time: number;
	    conns: Conn[];
	    hosts: Record<string, Host>;
	    listeners: Listener[];
	    me: Me;
	    rx: number;
	    tx: number;
	    scanMs: number;
	    method: string;
	    error?: string;
	    demo: boolean;
	    totals: Totals;
	    perConn: boolean;
	    perConnNote: string;
	    icmp: boolean;
	    icmpNote?: string;
	
	    static createFrom(source: any = {}) {
	        return new Snapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.time = source["time"];
	        this.conns = this.convertValues(source["conns"], Conn);
	        this.hosts = this.convertValues(source["hosts"], Host, true);
	        this.listeners = this.convertValues(source["listeners"], Listener);
	        this.me = this.convertValues(source["me"], Me);
	        this.rx = source["rx"];
	        this.tx = source["tx"];
	        this.scanMs = source["scanMs"];
	        this.method = source["method"];
	        this.error = source["error"];
	        this.demo = source["demo"];
	        this.totals = this.convertValues(source["totals"], Totals);
	        this.perConn = source["perConn"];
	        this.perConnNote = source["perConnNote"];
	        this.icmp = source["icmp"];
	        this.icmpNote = source["icmpNote"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace ping {
	
	export class Result {
	    sent: number;
	    received: number;
	    min: number;
	    avg: number;
	    max: number;
	    method: string;
	    at: number;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new Result(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sent = source["sent"];
	        this.received = source["received"];
	        this.min = source["min"];
	        this.avg = source["avg"];
	        this.max = source["max"];
	        this.method = source["method"];
	        this.at = source["at"];
	        this.error = source["error"];
	    }
	}

}

export namespace risk {
	
	export class Assessment {
	    score: number;
	    level: string;
	    reasons: string[];
	
	    static createFrom(source: any = {}) {
	        return new Assessment(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.score = source["score"];
	        this.level = source["level"];
	        this.reasons = source["reasons"];
	    }
	}
	export class ListStatus {
	    name: string;
	    entries: number;
	
	    static createFrom(source: any = {}) {
	        return new ListStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.entries = source["entries"];
	    }
	}
	export class Status {
	    lists: ListStatus[];
	    // Go type: time
	    updated: any;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.lists = this.convertValues(source["lists"], ListStatus);
	        this.updated = this.convertValues(source["updated"], null);
	        this.error = source["error"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace settings {
	
	export class Location {
	    mode: string;
	    lat: number;
	    lon: number;
	    label: string;
	
	    static createFrom(source: any = {}) {
	        return new Location(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mode = source["mode"];
	        this.lat = source["lat"];
	        this.lon = source["lon"];
	        this.label = source["label"];
	    }
	}
	export class Updates {
	    auto: boolean;
	    beta: boolean;
	    betaChosen: boolean;
	    skip: string;
	
	    static createFrom(source: any = {}) {
	        return new Updates(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.auto = source["auto"];
	        this.beta = source["beta"];
	        this.betaChosen = source["betaChosen"];
	        this.skip = source["skip"];
	    }
	}
	export class Settings {
	    location: Location;
	    intervalMs: number;
	    resolveNames: boolean;
	    showLocal: boolean;
	    logDays: number;
	    geoAuto: boolean;
	    pingHosts: boolean;
	    blocklists: boolean;
	    updates: Updates;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.location = this.convertValues(source["location"], Location);
	        this.intervalMs = source["intervalMs"];
	        this.resolveNames = source["resolveNames"];
	        this.showLocal = source["showLocal"];
	        this.logDays = source["logDays"];
	        this.geoAuto = source["geoAuto"];
	        this.pingHosts = source["pingHosts"];
	        this.blocklists = source["blocklists"];
	        this.updates = this.convertValues(source["updates"], Updates);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace update {
	
	export class Available {
	    version: string;
	    beta: boolean;
	    notes: string;
	    url: string;
	    size: number;
	
	    static createFrom(source: any = {}) {
	        return new Available(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.beta = source["beta"];
	        this.notes = source["notes"];
	        this.url = source["url"];
	        this.size = source["size"];
	    }
	}
	export class State {
	    current: string;
	    channel: string;
	    beta: boolean;
	    checking: boolean;
	    // Go type: time
	    lastCheck: any;
	    error?: string;
	    available?: Available;
	    downloading: boolean;
	    done: number;
	    total: number;
	    installing: boolean;
	    ready: boolean;
	    manual?: string;
	
	    static createFrom(source: any = {}) {
	        return new State(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.current = source["current"];
	        this.channel = source["channel"];
	        this.beta = source["beta"];
	        this.checking = source["checking"];
	        this.lastCheck = this.convertValues(source["lastCheck"], null);
	        this.error = source["error"];
	        this.available = this.convertValues(source["available"], Available);
	        this.downloading = source["downloading"];
	        this.done = source["done"];
	        this.total = source["total"];
	        this.installing = source["installing"];
	        this.ready = source["ready"];
	        this.manual = source["manual"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

