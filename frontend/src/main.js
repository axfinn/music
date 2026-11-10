import { createApp } from 'vue'

// Element Plus 的样式分两块来源：
//   1. 模板里用到的组件（<el-button> 等）—— 由 unplugin-vue-components 的
//      ElementPlusResolver 按需注入，见 vite.config.js；
//   2. 命令式 API（ElMessage / ElMessageBox）—— 模板扫描扫不到，
//      而业务代码是从 'element-plus' 整包 import 的，样式不会跟着走，
//      所以这里必须手动补上，否则弹层会没有样式。
import 'element-plus/theme-chalk/base.css'
import 'element-plus/theme-chalk/el-message.css'
import 'element-plus/theme-chalk/el-message-box.css'
import 'element-plus/theme-chalk/el-overlay.css'

import './styles/global.css'
import App from './App.vue'
import router from './router'

createApp(App).use(router).mount('#app')
