import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue({ template: { compilerOptions: { isCustomElement: tag => ['view', 'text', 'image'].includes(tag) } } })],
  test: {
    environment: 'jsdom',
    include: ['tests/**/*.test.{js,mjs}', 'src/**/*.test.{js,mjs}'],
    restoreMocks: true,
  },
})
