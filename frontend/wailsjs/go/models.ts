export namespace analyzer {

	export class TestConnectionResult {
	    type: string;
	    i18nMessage: string;

	    static createFrom(source: any = {}) {
	        return new TestConnectionResult(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.i18nMessage = source["i18nMessage"];
	    }
	}

}

export namespace cleaner {
	
	export class CleaningTaskSnapshot {
	    id: number;
	    // Go type: time
	    startTime: any;
	    path: string;
	    state: string;
	    errorMessage: string;
	    llmOutput: string;
	    scanProgress: scanner.ScanProgress;
	    stopping: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CleaningTaskSnapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.startTime = this.convertValues(source["startTime"], null);
	        this.path = source["path"];
	        this.state = source["state"];
	        this.errorMessage = source["errorMessage"];
	        this.llmOutput = source["llmOutput"];
	        this.scanProgress = this.convertValues(source["scanProgress"], scanner.ScanProgress);
	        this.stopping = source["stopping"];
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
	export class DeleteFailure {
	    path: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new DeleteFailure(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.message = source["message"];
	    }
	}

}

export namespace cleaningrecord {
	
	export class DiskUsage {
	    path: string;
	    size: number;
	    description: string;
	
	    static createFrom(source: any = {}) {
	        return new DiskUsage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.size = source["size"];
	        this.description = source["description"];
	    }
	}
	export class TrashFile {
	    name: string;
	    reason: string;
	    path: string;
	    size: number;
	    level: number;
	    isDeleted: boolean;
	
	    static createFrom(source: any = {}) {
	        return new TrashFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.reason = source["reason"];
	        this.path = source["path"];
	        this.size = source["size"];
	        this.level = source["level"];
	        this.isDeleted = source["isDeleted"];
	    }
	}
	export class CleaningRecord {
	    id: number;
	    // Go type: time
	    startTime: any;
	    freedSize: number;
	    trashSize: number;
	    trashFiles: TrashFile[];
	    topUsages: DiskUsage[];
	    path: string;
	    llmOutput: string;
	    tokenUsage: number;
	    state: string;
	    errorMessage: string;
	
	    static createFrom(source: any = {}) {
	        return new CleaningRecord(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.startTime = this.convertValues(source["startTime"], null);
	        this.freedSize = source["freedSize"];
	        this.trashSize = source["trashSize"];
	        this.trashFiles = this.convertValues(source["trashFiles"], TrashFile);
	        this.topUsages = this.convertValues(source["topUsages"], DiskUsage);
	        this.path = source["path"];
	        this.llmOutput = source["llmOutput"];
	        this.tokenUsage = source["tokenUsage"];
	        this.state = source["state"];
	        this.errorMessage = source["errorMessage"];
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

export namespace migration {
	
	export class Migration {
	    id: number;
	    name: string;
	    source: string;
	    dest: string;
	    // Go type: time
	    createdAt: any;
	
	    static createFrom(source: any = {}) {
	        return new Migration(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.source = source["source"];
	        this.dest = source["dest"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
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

export namespace model {
	
	export class DiskInfo {
	    name: string;
	    path: string;
	    total: number;
	    free: number;
	    used: number;
	
	    static createFrom(source: any = {}) {
	        return new DiskInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.total = source["total"];
	        this.free = source["free"];
	        this.used = source["used"];
	    }
	}

}

export namespace scanner {
	
	export class ScanProgress {
	    currentPath: string;
	    itemCount: number;
	    scannedSize: number;
	
	    static createFrom(source: any = {}) {
	        return new ScanProgress(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.currentPath = source["currentPath"];
	        this.itemCount = source["itemCount"];
	        this.scannedSize = source["scannedSize"];
	    }
	}

}

export namespace setting {
	
	export class Setting {
	    key: string;
	    value: string;
	
	    static createFrom(source: any = {}) {
	        return new Setting(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.value = source["value"];
	    }
	}

}

