<template>
  <view class="account-section">
    <view class="intro">
      <text class="page-title">我的账号</text>
      <text class="muted">个人资料与账号安全</text>
    </view>
    <view class="card profile-summary">
      <image v-if="user.avatar_url && !avatarFailed" class="avatar" :src="avatarUrl" mode="aspectFill" @error="$emit('avatar-error')" />
      <view v-else class="avatar avatar-fallback">{{ avatarInitial }}</view>
      <text class="display-name">{{ user.nickname || user.username }}</text>
      <text class="muted">@{{ user.username }}</text>
      <view class="account-details">
        <view class="detail-row"><text class="muted">所属企业</text><text class="detail-value">{{ tenant.name }}</text></view>
        <view class="detail-row"><text class="muted">账号角色</text><text>{{ user.role === 'admin' ? '企业管理员' : '企业成员' }}</text></view>
      </view>
    </view>
    <view class="card action-card">
      <button class="menu-button" :disabled="busy" @click="$emit('profile')"><text>编辑个人资料</text><text class="chevron">›</text></button>
      <button class="menu-button" :disabled="busy" @click="$emit('password')"><text>修改登录密码</text><text class="chevron">›</text></button>
    </view>
    <button class="secondary" :loading="busy" :disabled="busy" @click="$emit('logout')">退出登录</button>
  </view>
</template>
<script>
export default {
  props: { user: Object, tenant: Object, busy: Boolean, avatarFailed: Boolean, avatarUrl: String },
  emits: ['avatar-error', 'profile', 'password', 'logout'],
  computed: {
    avatarInitial() { return Array.from(this.user.nickname || this.user.username || '我')[0].toUpperCase() },
  },
}
</script>

<style scoped src="../account.css"></style>
