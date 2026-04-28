// src/router/index.ts
import * as VueRouter from "vue-router";
import {createMemoryHistory, createRouter} from "vue-router";
import Home from "@/views/Home.vue";
// NoArticleSelected is no longer needed as we're using query params approach
import ArticleReader from "@/components/layout/ArticleDetail.vue";
import ArticleList from "@/components/layout/ArticleList.vue";
import Settings from "@/views/Settings.vue";
import DigestList from "@/components/layout/DigestList.vue";
import DigestDetail from "@/components/layout/DigestDetail.vue";

const routes: VueRouter.RouteRecordRaw[] = [
    {
        path: '/',
        component: Home,
        children: [
            {
                path: '',
                redirect: '/all'
                // redirect: '/digests' // dev
            },
            {
                path: '/all',
                components: {
                    default: ArticleReader, // Changed to ArticleReader by default
                    list: ArticleList
                },
                props: {
                    default: route => ({ articleId: route.query.articleId?.toString() }), // Pass articleId from query params
                    list: route => ({
                        selectedArticleId: route.query.articleId?.toString(), // Pass articleId from query params
                        unreadOnly: route.query.unread === 'true',
                        bookmarkedOnly: route.query.bookmarked === 'true',
                        searchQuery: route.query.q
                    })
                }
            },
            {
                path: '/feed/:feedId',
                components: {
                    default: ArticleReader, // Changed to ArticleReader by default
                    list: ArticleList
                },
                props: {
                    default: route => ({ articleId: route.query.articleId?.toString() }), // Pass articleId from query params
                    list: route => ({
                        feedId: route.params.feedId,
                        selectedArticleId: route.query.articleId?.toString(), // Pass articleId from query params
                        unreadOnly: route.query.unread === 'true',
                        bookmarkedOnly: route.query.bookmarked === 'true',
                        searchQuery: route.query.q
                    })
                }
            },
            {
                path: '/digests',
                components: {
                    default: DigestDetail,
                    list: DigestList
                }
            },
            {
                path: '/digest/:id',
                components: {
                    default: DigestDetail,
                    list: DigestList
                },
                props: {
                    default: route => ({digestId: route.params.id}),
                    list: route => ({selectedDigestId: route.params.id})
                }
            }]
    },
    {
        path: '/settings',
        redirect: '/settings/feeds'
    },
    {
        path: '/settings/:tab',
        component: Settings,
    }
]

const router = createRouter({
    history: createMemoryHistory(),
    routes,
})

export default router;