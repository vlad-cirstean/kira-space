import '@workbench/workbench.css';
import 'cheetah-grid/main.css';
import './theme-alt.css';
import { createPinia } from 'pinia';
import { createApp } from 'vue';
import App from './App.vue';

createApp(App).use(createPinia()).mount('#app');
