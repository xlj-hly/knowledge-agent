import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router'
import { ConfigProvider } from 'antd'
import zhCN from 'antd/locale/zh_CN'
import './index.css'
import App from './App'

const root = document.getElementById('root')
if (!root) throw new Error('应用挂载节点不存在')

createRoot(root).render(
  <StrictMode>
    <ConfigProvider locale={zhCN} theme={{
      token: { colorPrimary: '#5362e8', borderRadius: 8, controlHeight: 38, fontFamily: "'Noto Sans SC', 'Microsoft YaHei', sans-serif" },
    }}>
      <BrowserRouter>
        <App />
      </BrowserRouter>
    </ConfigProvider>
  </StrictMode>,
)
