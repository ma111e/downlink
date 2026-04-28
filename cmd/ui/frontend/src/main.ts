import {createApp} from 'vue'
import './assets/index.css'
import App from './App.vue'
import router from "../router.ts";
import { createPinia } from 'pinia';


const app = createApp(App);

app
    .use(createPinia())
    .use(router)
    // .use(stores)
    .mount('#app')
