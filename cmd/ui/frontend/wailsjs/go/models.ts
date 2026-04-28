export namespace downlinkclient {
	
	export class ConnectionTestResult {
	    success: boolean;
	    message: string;
	    latency_ms: number;
	
	    static createFrom(source: any = {}) {
	        return new ConnectionTestResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.success = source["success"];
	        this.message = source["message"];
	        this.latency_ms = source["latency_ms"];
	    }
	}
	export class EnqueueOptions {
	    article_ids: string[];
	    provider_type?: string;
	    model_name?: string;
	    provider_name?: string;
	    fast_mode?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new EnqueueOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.article_ids = source["article_ids"];
	        this.provider_type = source["provider_type"];
	        this.model_name = source["model_name"];
	        this.provider_name = source["provider_name"];
	        this.fast_mode = source["fast_mode"];
	    }
	}
	export class QueueJobWithTitle {
	    id: string;
	    article_id: string;
	    article_title: string;
	    provider_type?: string;
	    model_name?: string;
	    provider_name?: string;
	
	    static createFrom(source: any = {}) {
	        return new QueueJobWithTitle(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.article_id = source["article_id"];
	        this.article_title = source["article_title"];
	        this.provider_type = source["provider_type"];
	        this.model_name = source["model_name"];
	        this.provider_name = source["provider_name"];
	    }
	}
	export class QueueStatus {
	    queue: QueueJobWithTitle[];
	    current_id: string;
	    current_title: string;
	    is_processing: boolean;
	
	    static createFrom(source: any = {}) {
	        return new QueueStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.queue = this.convertValues(source["queue"], QueueJobWithTitle);
	        this.current_id = source["current_id"];
	        this.current_title = source["current_title"];
	        this.is_processing = source["is_processing"];
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

export namespace emptypb {
	
	export class Empty {
	
	
	    static createFrom(source: any = {}) {
	        return new Empty(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	
	    }
	}

}

export namespace models {
	
	export class WorkerPoolConfig {
	    max_workers?: number;
	
	    static createFrom(source: any = {}) {
	        return new WorkerPoolConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.max_workers = source["max_workers"];
	    }
	}
	export class AnalysisConfig {
	    provider?: string;
	    persona?: string;
	    worker_pool?: WorkerPoolConfig;
	
	    static createFrom(source: any = {}) {
	        return new AnalysisConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.persona = source["persona"];
	        this.worker_pool = this.convertValues(source["worker_pool"], WorkerPoolConfig);
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
	export class RelatedArticle {
	    article_id: string;
	    related_article_id: string;
	    relation_type: string;
	    similarity_score: number;
	
	    static createFrom(source: any = {}) {
	        return new RelatedArticle(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.article_id = source["article_id"];
	        this.related_article_id = source["related_article_id"];
	        this.relation_type = source["relation_type"];
	        this.similarity_score = source["similarity_score"];
	    }
	}
	export class Category {
	    name: string;
	    color: string;
	    icon: string;
	
	    static createFrom(source: any = {}) {
	        return new Category(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.color = source["color"];
	        this.icon = source["icon"];
	    }
	}
	export class Tag {
	    id: string;
	    name: string;
	    color: string;
	    use_count?: number;
	
	    static createFrom(source: any = {}) {
	        return new Tag(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.color = source["color"];
	        this.use_count = source["use_count"];
	    }
	}
	export class Article {
	    id: string;
	    feed_id: string;
	    title: string;
	    content: string;
	    link: string;
	    published_at: time.Time;
	    fetched_at: time.Time;
	    read?: boolean;
	    tags: Tag[];
	    category_name?: string;
	    category?: Category;
	    hero_image?: string;
	    bookmarked?: boolean;
	    related_articles?: RelatedArticle[];
	    latest_importance_score?: number;
	
	    static createFrom(source: any = {}) {
	        return new Article(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.feed_id = source["feed_id"];
	        this.title = source["title"];
	        this.content = source["content"];
	        this.link = source["link"];
	        this.published_at = this.convertValues(source["published_at"], time.Time);
	        this.fetched_at = this.convertValues(source["fetched_at"], time.Time);
	        this.read = source["read"];
	        this.tags = this.convertValues(source["tags"], Tag);
	        this.category_name = source["category_name"];
	        this.category = this.convertValues(source["category"], Category);
	        this.hero_image = source["hero_image"];
	        this.bookmarked = source["bookmarked"];
	        this.related_articles = this.convertValues(source["related_articles"], RelatedArticle);
	        this.latest_importance_score = source["latest_importance_score"];
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
	export class ArticleAnalysis {
	    id: string;
	    article_id: string;
	    provider_type: string;
	    model_name: string;
	    importance_score: number;
	    key_points: string[];
	    insights: string[];
	    tldr: string;
	    justification: string;
	    brief_overview: string;
	    standard_synthesis: string;
	    comprehensive_synthesis: string;
	    thinking_process?: string;
	    raw_response: string;
	    created_at: time.Time;
	
	    static createFrom(source: any = {}) {
	        return new ArticleAnalysis(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.article_id = source["article_id"];
	        this.provider_type = source["provider_type"];
	        this.model_name = source["model_name"];
	        this.importance_score = source["importance_score"];
	        this.key_points = source["key_points"];
	        this.insights = source["insights"];
	        this.tldr = source["tldr"];
	        this.justification = source["justification"];
	        this.brief_overview = source["brief_overview"];
	        this.standard_synthesis = source["standard_synthesis"];
	        this.comprehensive_synthesis = source["comprehensive_synthesis"];
	        this.thinking_process = source["thinking_process"];
	        this.raw_response = source["raw_response"];
	        this.created_at = this.convertValues(source["created_at"], time.Time);
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
	export class ArticleCounts {
	    all_unread_count: number;
	    bookmarked_count: number;
	    unread_by_feed: Record<string, number>;
	
	    static createFrom(source: any = {}) {
	        return new ArticleCounts(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.all_unread_count = source["all_unread_count"];
	        this.bookmarked_count = source["bookmarked_count"];
	        this.unread_by_feed = source["unread_by_feed"];
	    }
	}
	export class ArticleFilter {
	    unread_only: boolean;
	    category_name: string;
	    tag_id: string;
	    bookmarked_only: boolean;
	    related_to_id: string;
	    start_date?: time.Time;
	    end_date?: time.Time;
	    feed_id: string;
	    exclude_digested?: boolean;
	    offset?: number;
	    limit?: number;
	    query?: string;
	
	    static createFrom(source: any = {}) {
	        return new ArticleFilter(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.unread_only = source["unread_only"];
	        this.category_name = source["category_name"];
	        this.tag_id = source["tag_id"];
	        this.bookmarked_only = source["bookmarked_only"];
	        this.related_to_id = source["related_to_id"];
	        this.start_date = this.convertValues(source["start_date"], time.Time);
	        this.end_date = this.convertValues(source["end_date"], time.Time);
	        this.feed_id = source["feed_id"];
	        this.exclude_digested = source["exclude_digested"];
	        this.offset = source["offset"];
	        this.limit = source["limit"];
	        this.query = source["query"];
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
	export class ArticleUpdate {
	    read?: boolean;
	    tag_ids?: string[];
	    category_name?: string;
	    hero_image?: string;
	    bookmarked?: boolean;
	    related_articles?: RelatedArticle[];
	
	    static createFrom(source: any = {}) {
	        return new ArticleUpdate(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.read = source["read"];
	        this.tag_ids = source["tag_ids"];
	        this.category_name = source["category_name"];
	        this.hero_image = source["hero_image"];
	        this.bookmarked = source["bookmarked"];
	        this.related_articles = this.convertValues(source["related_articles"], RelatedArticle);
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
	
	export class DigestAnalysis {
	    digest_id: string;
	    analysis_id: string;
	    article_id: string;
	    duplicate_group?: string;
	    is_most_comprehensive: boolean;
	    analysis?: ArticleAnalysis;
	
	    static createFrom(source: any = {}) {
	        return new DigestAnalysis(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.digest_id = source["digest_id"];
	        this.analysis_id = source["analysis_id"];
	        this.article_id = source["article_id"];
	        this.duplicate_group = source["duplicate_group"];
	        this.is_most_comprehensive = source["is_most_comprehensive"];
	        this.analysis = this.convertValues(source["analysis"], ArticleAnalysis);
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
	export class DigestProviderResult {
	    id: string;
	    digest_id: string;
	    provider_type: string;
	    model_name: string;
	    brief_overview: string;
	    standard_synthesis: string;
	    comprehensive_synthesis: string;
	    processing_time: number;
	    error: string;
	    created_at: time.Time;
	
	    static createFrom(source: any = {}) {
	        return new DigestProviderResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.digest_id = source["digest_id"];
	        this.provider_type = source["provider_type"];
	        this.model_name = source["model_name"];
	        this.brief_overview = source["brief_overview"];
	        this.standard_synthesis = source["standard_synthesis"];
	        this.comprehensive_synthesis = source["comprehensive_synthesis"];
	        this.processing_time = source["processing_time"];
	        this.error = source["error"];
	        this.created_at = this.convertValues(source["created_at"], time.Time);
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
	export class Digest {
	    id: string;
	    created_at: time.Time;
	    article_count?: number;
	    time_window: number;
	    raw_grouping_response?: string;
	    digest_summary?: string;
	    provider_results: DigestProviderResult[];
	    digest_analyses?: DigestAnalysis[];
	
	    static createFrom(source: any = {}) {
	        return new Digest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.created_at = this.convertValues(source["created_at"], time.Time);
	        this.article_count = source["article_count"];
	        this.time_window = source["time_window"];
	        this.raw_grouping_response = source["raw_grouping_response"];
	        this.digest_summary = source["digest_summary"];
	        this.provider_results = this.convertValues(source["provider_results"], DigestProviderResult);
	        this.digest_analyses = this.convertValues(source["digest_analyses"], DigestAnalysis);
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
	
	
	export class DiscordNotificationConfig {
	    enabled: boolean;
	    webhook_url: string;
	
	    static createFrom(source: any = {}) {
	        return new DiscordNotificationConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.webhook_url = source["webhook_url"];
	    }
	}
	export class Feed {
	    id: string;
	    url: string;
	    type: string;
	    title: string;
	    last_fetch: time.Time;
	    scraper?: Record<string, any>;
	    enabled?: boolean;
	    group_id?: string;
	
	    static createFrom(source: any = {}) {
	        return new Feed(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.url = source["url"];
	        this.type = source["type"];
	        this.title = source["title"];
	        this.last_fetch = this.convertValues(source["last_fetch"], time.Time);
	        this.scraper = source["scraper"];
	        this.enabled = source["enabled"];
	        this.group_id = source["group_id"];
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
	export class Selectors {
	    article?: string;
	    cutoff?: string;
	    blacklist?: string;
	
	    static createFrom(source: any = {}) {
	        return new Selectors(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.article = source["article"];
	        this.cutoff = source["cutoff"];
	        this.blacklist = source["blacklist"];
	    }
	}
	export class FeedConfig {
	    url: string;
	    title?: string;
	    type: string;
	    enabled: boolean;
	    scraper?: Record<string, any>;
	    scraping?: string;
	    selectors?: Selectors;
	
	    static createFrom(source: any = {}) {
	        return new FeedConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.title = source["title"];
	        this.type = source["type"];
	        this.enabled = source["enabled"];
	        this.scraper = source["scraper"];
	        this.scraping = source["scraping"];
	        this.selectors = this.convertValues(source["selectors"], Selectors);
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
	export class GitHubPagesNotificationConfig {
	    enabled: boolean;
	    repo_url: string;
	    branch: string;
	    configure_pages: boolean;
	    token: string;
	    output_dir: string;
	    base_url: string;
	    commit_author: string;
	    commit_email: string;
	    clone_dir: string;
	    discord_webhook_url: string;
	
	    static createFrom(source: any = {}) {
	        return new GitHubPagesNotificationConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.repo_url = source["repo_url"];
	        this.branch = source["branch"];
	        this.configure_pages = source["configure_pages"];
	        this.token = source["token"];
	        this.output_dir = source["output_dir"];
	        this.base_url = source["base_url"];
	        this.commit_author = source["commit_author"];
	        this.commit_email = source["commit_email"];
	        this.clone_dir = source["clone_dir"];
	        this.discord_webhook_url = source["discord_webhook_url"];
	    }
	}
	export class ModelInfo {
	    id: string;
	    name: string;
	    display_name?: string;
	    description?: string;
	    provider_type: string;
	
	    static createFrom(source: any = {}) {
	        return new ModelInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.display_name = source["display_name"];
	        this.description = source["description"];
	        this.provider_type = source["provider_type"];
	    }
	}
	export class ModelsResponse {
	    models: ModelInfo[];
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new ModelsResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.models = this.convertValues(source["models"], ModelInfo);
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
	export class NotificationsConfig {
	    discord: DiscordNotificationConfig;
	    github_pages: GitHubPagesNotificationConfig;
	
	    static createFrom(source: any = {}) {
	        return new NotificationsConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.discord = this.convertValues(source["discord"], DiscordNotificationConfig);
	        this.github_pages = this.convertValues(source["github_pages"], GitHubPagesNotificationConfig);
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
	export class ProviderConfig {
	    name: string;
	    provider_type: string;
	    model_name: string;
	    enabled: boolean;
	    base_url?: string;
	    temperature?: number;
	    max_retries?: number;
	    timeout_minutes?: number;
	    api_key?: string;
	
	    static createFrom(source: any = {}) {
	        return new ProviderConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.provider_type = source["provider_type"];
	        this.model_name = source["model_name"];
	        this.enabled = source["enabled"];
	        this.base_url = source["base_url"];
	        this.temperature = source["temperature"];
	        this.max_retries = source["max_retries"];
	        this.timeout_minutes = source["timeout_minutes"];
	        this.api_key = source["api_key"];
	    }
	}
	
	
	export class ServerConfig {
	    feeds: FeedConfig[];
	    db_path: string;
	    providers: ProviderConfig[];
	    analysis: AnalysisConfig;
	    notifications: NotificationsConfig;
	    solimen_addr: string;
	
	    static createFrom(source: any = {}) {
	        return new ServerConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.feeds = this.convertValues(source["feeds"], FeedConfig);
	        this.db_path = source["db_path"];
	        this.providers = this.convertValues(source["providers"], ProviderConfig);
	        this.analysis = this.convertValues(source["analysis"], AnalysisConfig);
	        this.notifications = this.convertValues(source["notifications"], NotificationsConfig);
	        this.solimen_addr = source["solimen_addr"];
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

export namespace protos {
	
	export class RefreshFeedResponse {
	    feed_id?: string;
	    feed_title?: string;
	    total_fetched?: number;
	    stored?: number;
	    skipped?: number;
	    errors?: string[];
	
	    static createFrom(source: any = {}) {
	        return new RefreshFeedResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.feed_id = source["feed_id"];
	        this.feed_title = source["feed_title"];
	        this.total_fetched = source["total_fetched"];
	        this.stored = source["stored"];
	        this.skipped = source["skipped"];
	        this.errors = source["errors"];
	    }
	}

}

export namespace time {
	
	export class Time {
	
	
	    static createFrom(source: any = {}) {
	        return new Time(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	
	    }
	}

}
