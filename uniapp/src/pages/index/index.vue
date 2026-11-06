<template>
  <view class="page">
    <view class="brand-row">
      <view class="brand-mark">企</view>
      <view class="brand-copy">
        <text class="eyebrow">企业账号中心</text>
        <text class="brand-name">{{ account.tenant ? account.tenant.name : '企业服务' }}</text>
      </view>
    </view>
    <view v-if="account.notice" class="notice" role="status">{{ account.notice }}</view>
    <view v-if="account.error" class="error" role="alert">{{ account.error }}</view>
    <view v-if="!account.ready" class="card state-card">
      <text class="heading">{{ account.loading ? '正在连接企业' : '暂时无法进入' }}</text>
      <text class="muted">{{ account.loading ? '正在确认企业信息与登录状态…' : '请确认网络和企业入口后重试。' }}</text>
      <button v-if="!account.loading" class="primary" @click="retry">重新连接</button>
    </view>
    <WechatLogin v-else-if="!account.user && wechatLogin" :tenant="account.tenant" :busy="account.busy" @login="controller.loginWithWechat" />
    <AuthForm v-else-if="!account.user" :tenant="account.tenant" :screen="account.screen" :busy="account.busy" :draft="account.credentials"
      @screen="controller.switchScreen" @submit="controller.submitCredentials" @update="controller.updateCredentials" />
    <AccountHome v-else-if="account.screen === 'home'" :user="account.user" :tenant="account.tenant" :busy="account.busy" :avatar-failed="account.avatarFailed"
      :avatar-url="controller.avatarURL(account.user.avatar_url)"
      @profile="controller.openProfile" @password="controller.switchScreen('password')" @logout="controller.logout" @avatar-error="controller.avatarError" />
    <ProfileForm v-else-if="account.screen === 'profile'" :user="account.user" :busy="account.busy" :draft="account.profile" :wechat="wechatLogin" :avatar-url="controller.avatarURL(account.profile.avatar_url)"
      @avatar="controller.uploadAvatar" @avatar-error="controller.profileError" @submit="controller.saveProfile" @back="controller.switchScreen('home')" @update="controller.updateProfile" />
    <PasswordForm v-else-if="account.screen === 'password'" :busy="account.busy" :draft="account.passwords" :wechat="wechatLogin"
      @submit="controller.changePassword" @back="controller.switchScreen('home')" @update="controller.updatePasswords" />
    <view class="footer"><text>企业账号服务</text><text>安全连接 · 个人账户</text></view>
  </view>
</template>

<script>
import AuthForm from './components/AuthForm.vue'
import WechatLogin from './components/WechatLogin.vue'
import AccountHome from './components/AccountHome.vue'
import ProfileForm from './components/ProfileForm.vue'
import PasswordForm from './components/PasswordForm.vue'
import { createTenantClient, resolveApiConfig } from '../../api/request.mjs'
import { createWechatLogin } from '../../api/wechat.mjs'
import { createAccountController, createAccountState } from './state.mjs'

export default {
  components: { AuthForm, WechatLogin, AccountHome, ProfileForm, PasswordForm },
  data() {
    return { account: createAccountState(), controller: null, wechatLogin: process.env.UNI_PLATFORM === 'mp-weixin' }
  },
  onLoad() {
    this.initialize()
  },
  onShow() {
    this.controller?.resume()
  },
  onUnload() {
    this.controller?.cancelPending()
  },
  methods: {
    retry() {
      if (this.controller) this.controller.refresh()
      else this.initialize()
    },
    initialize() {
      this.controller?.cancelPending()
      this.controller = null
      this.account = createAccountState()
      try {
        let browserHostname = ''
        // #ifdef H5
        if (!this.wechatLogin) browserHostname = globalThis.location?.hostname || ''
        // #endif
        const config = resolveApiConfig(import.meta.env, browserHostname)
        const client = createTenantClient({ uni, ...config, onUnauthorized: error => this.controller?.sessionExpired(error) })
        const wechat = process.env.UNI_PLATFORM === 'mp-weixin' ? createWechatLogin(uni) : null
        this.controller = createAccountController({ state: this.account, client, wechat })
        this.controller.refresh()
      } catch (error) {
        this.account.loading = false
        this.account.ready = false
        this.account.error = error.message
      }
    },
  },
}
</script>

<style scoped src="./account.css"></style>
