<template>
  <view class="intro"><text class="page-title">编辑个人资料</text><text class="muted">用户名固定，昵称与头像可随时更新</text></view>
  <form class="card" @submit="submit">
    <view class="field"><text class="label">用户名</text><view class="readonly">{{ user.username }}</view></view>
    <view class="field">
      <label class="label" for="nickname">昵称</label>
      <input id="nickname" name="nickname" :type="wechat ? 'nickname' : 'text'" :value="draft.nickname" @input="update('nickname', $event.detail.value)" @blur="update('nickname', $event.detail.value)" class="input" placeholder="请输入昵称" :maxlength="-1" :disabled="busy" />
      <text class="hint">{{ wechat ? '输入时可选择微信昵称，最多 64 个字符' : '最多 64 个字符，留空显示用户名' }}</text>
    </view>
    <view class="field">
      <text class="label">头像</text>
      <image v-if="avatarUrl" class="avatar" :src="avatarUrl" mode="aspectFill" />
      <button v-if="wechat" class="secondary" open-type="chooseAvatar" :disabled="busy" @chooseavatar="chooseAvatar">使用微信头像</button>
      <button v-else class="secondary" :disabled="busy || selecting" @click="chooseImage">选择头像</button>
      <text class="hint">支持 JPG、PNG 图片，最大 2MB；保存资料后生效</text>
    </view>
    <button class="primary" form-type="submit" :loading="busy" :disabled="busy">保存资料</button>
    <button class="text-button" :disabled="busy" @click="$emit('back')">返回我的账号</button>
  </form>
</template>

<script>
export default {
  props: { user: Object, busy: Boolean, draft: Object, wechat: Boolean, avatarUrl: String },
  emits: ['back', 'submit', 'update', 'avatar', 'avatar-error'],
  data() { return { selecting: false, active: true } },
  beforeUnmount() { this.active = false },
  methods: {
    update(field, value) { this.$emit('update', { ...this.draft, [field]: value }) },
    submit(event) {
      // 微信昵称在 form submit 时收集，避免异步校验清空后仍提交旧草稿。
      const nickname = event?.detail?.value?.nickname
      if (typeof nickname === 'string') this.update('nickname', nickname)
      this.$emit('submit')
    },
    chooseAvatar(event) {
      const path = event?.detail?.avatarUrl
      if (!this.busy && typeof path === 'string' && path) this.$emit('avatar', path)
    },
    chooseImage() {
      if (this.busy || this.selecting) return
      this.selecting = true
      const finish = () => { if (this.active) this.selecting = false }
      try {
        uni.chooseImage({
          count: 1,
          sizeType: ['compressed'],
          success: result => {
            if (!this.active) return
            finish()
            const path = result?.tempFilePaths?.[0]
            if (typeof path === 'string' && path) this.$emit('avatar', path)
            else this.$emit('avatar-error', '图片选择结果无效，请重试。')
          },
          fail: error => {
            if (!this.active) return
            finish()
            if (!error?.errMsg?.includes('cancel')) this.$emit('avatar-error', '无法选择图片，请重试。')
          },
        })
      } catch {
        finish()
        this.$emit('avatar-error', '无法选择图片，请重试。')
      }
    },
  },
}
</script>

<style scoped src="../account.css"></style>
