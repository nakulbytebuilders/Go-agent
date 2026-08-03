export namespace models {
	
	export class AppActivity {
	    id: number;
	    app_name: string;
	    window_title: string;
	    pid: number;
	    duration_sec: number;
	    // Go type: time
	    start_time: any;
	    // Go type: time
	    end_time: any;
	    // Go type: time
	    created_at: any;
	
	    static createFrom(source: any = {}) {
	        return new AppActivity(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.app_name = source["app_name"];
	        this.window_title = source["window_title"];
	        this.pid = source["pid"];
	        this.duration_sec = source["duration_sec"];
	        this.start_time = this.convertValues(source["start_time"], null);
	        this.end_time = this.convertValues(source["end_time"], null);
	        this.created_at = this.convertValues(source["created_at"], null);
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
	export class AppUsageStat {
	    app_name: string;
	    total_duration_sec: number;
	    launch_count: number;
	    // Go type: time
	    last_active: any;
	
	    static createFrom(source: any = {}) {
	        return new AppUsageStat(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.app_name = source["app_name"];
	        this.total_duration_sec = source["total_duration_sec"];
	        this.launch_count = source["launch_count"];
	        this.last_active = this.convertValues(source["last_active"], null);
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
	export class BrowserActivity {
	    id: number;
	    browser_name: string;
	    tab_title: string;
	    domain: string;
	    url: string;
	    duration_sec: number;
	    // Go type: time
	    start_time: any;
	    // Go type: time
	    end_time: any;
	    // Go type: time
	    created_at: any;
	
	    static createFrom(source: any = {}) {
	        return new BrowserActivity(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.browser_name = source["browser_name"];
	        this.tab_title = source["tab_title"];
	        this.domain = source["domain"];
	        this.url = source["url"];
	        this.duration_sec = source["duration_sec"];
	        this.start_time = this.convertValues(source["start_time"], null);
	        this.end_time = this.convertValues(source["end_time"], null);
	        this.created_at = this.convertValues(source["created_at"], null);
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
	export class BrowserUsageStat {
	    domain: string;
	    total_duration_sec: number;
	    visit_count: number;
	    // Go type: time
	    last_visited: any;
	
	    static createFrom(source: any = {}) {
	        return new BrowserUsageStat(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.domain = source["domain"];
	        this.total_duration_sec = source["total_duration_sec"];
	        this.visit_count = source["visit_count"];
	        this.last_visited = this.convertValues(source["last_visited"], null);
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
	export class ScreenshotRecord {
	    id: number;
	    file_path: string;
	    file_size: number;
	    width: number;
	    height: number;
	    // Go type: time
	    captured_at: any;
	    sync_status: string;
	
	    static createFrom(source: any = {}) {
	        return new ScreenshotRecord(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.file_path = source["file_path"];
	        this.file_size = source["file_size"];
	        this.width = source["width"];
	        this.height = source["height"];
	        this.captured_at = this.convertValues(source["captured_at"], null);
	        this.sync_status = source["sync_status"];
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
	export class UnifiedActivity {
	    id: number;
	    category: string;
	    name: string;
	    title: string;
	    domain: string;
	    pid: number;
	    duration_sec: number;
	    // Go type: time
	    start_time: any;
	    // Go type: time
	    end_time: any;
	    // Go type: time
	    created_at: any;
	
	    static createFrom(source: any = {}) {
	        return new UnifiedActivity(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.category = source["category"];
	        this.name = source["name"];
	        this.title = source["title"];
	        this.domain = source["domain"];
	        this.pid = source["pid"];
	        this.duration_sec = source["duration_sec"];
	        this.start_time = this.convertValues(source["start_time"], null);
	        this.end_time = this.convertValues(source["end_time"], null);
	        this.created_at = this.convertValues(source["created_at"], null);
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

