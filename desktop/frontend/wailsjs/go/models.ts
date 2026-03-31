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
	export class BackupInfoDTO {
	    name: string;
	    date: string;
	    size: number;
	
	    static createFrom(source: any = {}) {
	        return new BackupInfoDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.date = source["date"];
	        this.size = source["size"];
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
	    time: string;

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
	        this.time = source["time"];
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
	export class FlashcardDTO {
	    front: string;
	    back: string;
	    id: string;
	
	    static createFrom(source: any = {}) {
	        return new FlashcardDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.front = source["front"];
	        this.back = source["back"];
	        this.id = source["id"];
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
	
	
	export class LinkSuggestionDTO {
	    target: string;
	    context: string;
	    line: number;
	
	    static createFrom(source: any = {}) {
	        return new LinkSuggestionDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.target = source["target"];
	        this.context = source["context"];
	        this.line = source["line"];
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
	export class NoteVersionDTO {
	    hash: string;
	    date: string;
	    message: string;
	    author: string;
	
	    static createFrom(source: any = {}) {
	        return new NoteVersionDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hash = source["hash"];
	        this.date = source["date"];
	        this.message = source["message"];
	        this.author = source["author"];
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
	export class PluginInfoDTO {
	    name: string;
	    description: string;
	    version: string;
	    author: string;
	    enabled: boolean;
	    commands: string[];
	    hooks: string[];
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new PluginInfoDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.version = source["version"];
	        this.author = source["author"];
	        this.enabled = source["enabled"];
	        this.commands = source["commands"];
	        this.hooks = source["hooks"];
	        this.path = source["path"];
	    }
	}
	export class ProjectMilestone {
	    text: string;
	    done: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ProjectMilestone(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.text = source["text"];
	        this.done = source["done"];
	    }
	}
	export class ProjectGoal {
	    title: string;
	    done: boolean;
	    milestones: ProjectMilestone[];
	
	    static createFrom(source: any = {}) {
	        return new ProjectGoal(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.done = source["done"];
	        this.milestones = this.convertValues(source["milestones"], ProjectMilestone);
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
	export class Project {
	    name: string;
	    description: string;
	    folder: string;
	    tags: string[];
	    status: string;
	    color: string;
	    createdAt: string;
	    notes: string[];
	    taskFilter: string;
	    category: string;
	    goals: ProjectGoal[];
	    nextAction: string;
	    priority: number;
	    dueDate: string;
	    timeSpent: number;
	
	    static createFrom(source: any = {}) {
	        return new Project(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.folder = source["folder"];
	        this.tags = source["tags"];
	        this.status = source["status"];
	        this.color = source["color"];
	        this.createdAt = source["createdAt"];
	        this.notes = source["notes"];
	        this.taskFilter = source["taskFilter"];
	        this.category = source["category"];
	        this.goals = this.convertValues(source["goals"], ProjectGoal);
	        this.nextAction = source["nextAction"];
	        this.priority = source["priority"];
	        this.dueDate = source["dueDate"];
	        this.timeSpent = source["timeSpent"];
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
	
	
	export class QuizQuestionDTO {
	    question: string;
	    choices: string[];
	    answer: number;
	    source: string;
	
	    static createFrom(source: any = {}) {
	        return new QuizQuestionDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.question = source["question"];
	        this.choices = source["choices"];
	        this.answer = source["answer"];
	        this.source = source["source"];
	    }
	}
	export class RecurringTaskDTO {
	    text: string;
	    pattern: string;
	    notePath: string;
	    line: number;
	    nextDue: string;
	
	    static createFrom(source: any = {}) {
	        return new RecurringTaskDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.text = source["text"];
	        this.pattern = source["pattern"];
	        this.notePath = source["notePath"];
	        this.line = source["line"];
	        this.nextDue = source["nextDue"];
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
	    description?: string;
	
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
	        this.description = source["description"];
	    }
	}
	export class SmartConnectionDTO {
	    relPath: string;
	    title: string;
	    score: number;
	    reason: string;
	
	    static createFrom(source: any = {}) {
	        return new SmartConnectionDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.relPath = source["relPath"];
	        this.title = source["title"];
	        this.score = source["score"];
	        this.reason = source["reason"];
	    }
	}
	export class SnippetDTO {
	    trigger: string;
	    content: string;
	    description: string;
	
	    static createFrom(source: any = {}) {
	        return new SnippetDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.trigger = source["trigger"];
	        this.content = source["content"];
	        this.description = source["description"];
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
	export class TableDataDTO {
	    headers: string[];
	    rows: string[][];
	    startLine: number;
	    endLine: number;
	
	    static createFrom(source: any = {}) {
	        return new TableDataDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.headers = source["headers"];
	        this.rows = source["rows"];
	        this.startLine = source["startLine"];
	        this.endLine = source["endLine"];
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
	export class TaskItem {
	    text: string;
	    done: boolean;
	    notePath: string;
	    lineNum: number;
	    priority: number;
	    dueDate: string;
	    tags: string[];
	    estimatedMinutes: number;
	    scheduledTime: string;
	    recurrence: string;
	    goalId: string;
	    snoozedUntil: string;

	    static createFrom(source: any = {}) {
	        return new TaskItem(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.text = source["text"];
	        this.done = source["done"];
	        this.notePath = source["notePath"];
	        this.lineNum = source["lineNum"];
	        this.priority = source["priority"];
	        this.dueDate = source["dueDate"];
	        this.tags = source["tags"];
	        this.estimatedMinutes = source["estimatedMinutes"];
	        this.scheduledTime = source["scheduledTime"];
	        this.recurrence = source["recurrence"];
	        this.goalId = source["goalId"];
	        this.snoozedUntil = source["snoozedUntil"];
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
	export class TimelineEntryDTO {
	    date: string;
	    title: string;
	    relPath: string;
	    tags: string[];
	    wordCount: number;
	
	    static createFrom(source: any = {}) {
	        return new TimelineEntryDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.date = source["date"];
	        this.title = source["title"];
	        this.relPath = source["relPath"];
	        this.tags = source["tags"];
	        this.wordCount = source["wordCount"];
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

