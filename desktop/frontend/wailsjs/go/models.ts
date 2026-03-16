export namespace main {
	
	export class BacklinkContextEntry {
	    relPath: string;
	    title: string;
	    context: string;
	
	    static createFrom(source: any = {}) {
	        return new BacklinkContextEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.relPath = source["relPath"];
	        this.title = source["title"];
	        this.context = source["context"];
	    }
	}
	export class BookmarkFile {
	    starred: string[];
	    recent: string[];
	
	    static createFrom(source: any = {}) {
	        return new BookmarkFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.starred = source["starred"];
	        this.recent = source["recent"];
	    }
	}
	export class BotInfo {
	    kind: number;
	    name: string;
	    desc: string;
	    icon: string;
	
	    static createFrom(source: any = {}) {
	        return new BotInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.name = source["name"];
	        this.desc = source["desc"];
	        this.icon = source["icon"];
	    }
	}
	export class BotResultData {
	    response: string;
	    tags?: string[];
	    links?: string[];
	
	    static createFrom(source: any = {}) {
	        return new BotResultData(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.response = source["response"];
	        this.tags = source["tags"];
	        this.links = source["links"];
	    }
	}
	export class CalendarEventDTO {
	    title: string;
	    date: string;
	    endDate: string;
	    location: string;
	    allDay: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CalendarEventDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.date = source["date"];
	        this.endDate = source["endDate"];
	        this.location = source["location"];
	        this.allDay = source["allDay"];
	    }
	}
	export class CalendarData {
	    year: number;
	    month: number;
	    dailyNotes: string[];
	    tasks: Record<string, Array<CalendarTask>>;
	    events: CalendarEventDTO[];
	
	    static createFrom(source: any = {}) {
	        return new CalendarData(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.year = source["year"];
	        this.month = source["month"];
	        this.dailyNotes = source["dailyNotes"];
	        this.tasks = this.convertValues(source["tasks"], Array<CalendarTask>, true);
	        this.events = this.convertValues(source["events"], CalendarEventDTO);
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
	
	export class CalendarTask {
	    text: string;
	    done: boolean;
	    notePath: string;
	    priority: number;
	    lineNum: number;
	
	    static createFrom(source: any = {}) {
	        return new CalendarTask(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.text = source["text"];
	        this.done = source["done"];
	        this.notePath = source["notePath"];
	        this.priority = source["priority"];
	        this.lineNum = source["lineNum"];
	    }
	}
	export class CommandInfo {
	    action: string;
	    label: string;
	    desc: string;
	    shortcut: string;
	    icon: string;
	
	    static createFrom(source: any = {}) {
	        return new CommandInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.action = source["action"];
	        this.label = source["label"];
	        this.desc = source["desc"];
	        this.shortcut = source["shortcut"];
	        this.icon = source["icon"];
	    }
	}
	export class FolderNode {
	    name: string;
	    path: string;
	    isFolder: boolean;
	    children?: FolderNode[];
	
	    static createFrom(source: any = {}) {
	        return new FolderNode(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.isFolder = source["isFolder"];
	        this.children = this.convertValues(source["children"], FolderNode);
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
	export class GraphEdge {
	    source: string;
	    target: string;
	
	    static createFrom(source: any = {}) {
	        return new GraphEdge(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.source = source["source"];
	        this.target = source["target"];
	    }
	}
	export class GraphNode {
	    id: string;
	    name: string;
	    incoming: number;
	    outgoing: number;
	    total: number;
	    isCenter: boolean;
	    hopDist: number;
	
	    static createFrom(source: any = {}) {
	        return new GraphNode(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.incoming = source["incoming"];
	        this.outgoing = source["outgoing"];
	        this.total = source["total"];
	        this.isCenter = source["isCenter"];
	        this.hopDist = source["hopDist"];
	    }
	}
	export class GraphData {
	    nodes: GraphNode[];
	    edges: GraphEdge[];
	    // Go type: struct { TotalNodes int "json:\"totalNodes\""; TotalEdges int "json:\"totalEdges\""; OrphanCount int "json:\"orphanCount\"" }
	    stats: any;
	
	    static createFrom(source: any = {}) {
	        return new GraphData(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.nodes = this.convertValues(source["nodes"], GraphNode);
	        this.edges = this.convertValues(source["edges"], GraphEdge);
	        this.stats = this.convertValues(source["stats"], Object);
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
	
	
	export class NoteDetail {
	    relPath: string;
	    title: string;
	    content: string;
	    frontmatter: Record<string, any>;
	    links: string[];
	    backlinks: string[];
	    modTime: string;
	    wordCount: number;
	
	    static createFrom(source: any = {}) {
	        return new NoteDetail(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.relPath = source["relPath"];
	        this.title = source["title"];
	        this.content = source["content"];
	        this.frontmatter = source["frontmatter"];
	        this.links = source["links"];
	        this.backlinks = source["backlinks"];
	        this.modTime = source["modTime"];
	        this.wordCount = source["wordCount"];
	    }
	}
	export class NoteInfo {
	    relPath: string;
	    title: string;
	    modTime: string;
	    size: number;
	
	    static createFrom(source: any = {}) {
	        return new NoteInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.relPath = source["relPath"];
	        this.title = source["title"];
	        this.modTime = source["modTime"];
	        this.size = source["size"];
	    }
	}
	export class OutlineItem {
	    level: number;
	    text: string;
	    line: number;
	
	    static createFrom(source: any = {}) {
	        return new OutlineItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.level = source["level"];
	        this.text = source["text"];
	        this.line = source["line"];
	    }
	}
	export class SearchHit {
	    relPath: string;
	    title: string;
	    line: number;
	    column: number;
	    matchLine: string;
	    score: number;
	
	    static createFrom(source: any = {}) {
	        return new SearchHit(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.relPath = source["relPath"];
	        this.title = source["title"];
	        this.line = source["line"];
	        this.column = source["column"];
	        this.matchLine = source["matchLine"];
	        this.score = source["score"];
	    }
	}
	export class SettingItem {
	    key: string;
	    label: string;
	    type: string;
	    value: any;
	    options?: string[];
	    category: string;
	
	    static createFrom(source: any = {}) {
	        return new SettingItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.label = source["label"];
	        this.type = source["type"];
	        this.value = source["value"];
	        this.options = source["options"];
	        this.category = source["category"];
	    }
	}
	export class StatEntry {
	    name: string;
	    value: number;
	
	    static createFrom(source: any = {}) {
	        return new StatEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.value = source["value"];
	    }
	}
	export class TagEntryDTO {
	    name: string;
	    count: number;
	
	    static createFrom(source: any = {}) {
	        return new TagEntryDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.count = source["count"];
	    }
	}
	export class TemplateInfo {
	    name: string;
	    content: string;
	    isUser: boolean;
	
	    static createFrom(source: any = {}) {
	        return new TemplateInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.content = source["content"];
	        this.isUser = source["isUser"];
	    }
	}
	export class TrashItemInfo {
	    origPath: string;
	    trashFile: string;
	    deletedAt: string;
	    timeAgo: string;
	
	    static createFrom(source: any = {}) {
	        return new TrashItemInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.origPath = source["origPath"];
	        this.trashFile = source["trashFile"];
	        this.deletedAt = source["deletedAt"];
	        this.timeAgo = source["timeAgo"];
	    }
	}
	export class VaultListEntry {
	    name: string;
	    path: string;
	    lastOpen: string;
	
	    static createFrom(source: any = {}) {
	        return new VaultListEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.lastOpen = source["lastOpen"];
	    }
	}
	export class VaultStatsData {
	    totalNotes: number;
	    totalWords: number;
	    totalLinks: number;
	    totalBacklinks: number;
	    uniqueTagCount: number;
	    orphanNotes: number;
	    avgLinks: number;
	    topLinked: StatEntry[];
	    largestNotes: StatEntry[];
	    topTags: StatEntry[];
	
	    static createFrom(source: any = {}) {
	        return new VaultStatsData(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.totalNotes = source["totalNotes"];
	        this.totalWords = source["totalWords"];
	        this.totalLinks = source["totalLinks"];
	        this.totalBacklinks = source["totalBacklinks"];
	        this.uniqueTagCount = source["uniqueTagCount"];
	        this.orphanNotes = source["orphanNotes"];
	        this.avgLinks = source["avgLinks"];
	        this.topLinked = this.convertValues(source["topLinked"], StatEntry);
	        this.largestNotes = this.convertValues(source["largestNotes"], StatEntry);
	        this.topTags = this.convertValues(source["topTags"], StatEntry);
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

