<script setup>
import { ref, computed, onMounted, watch, inject } from "vue";
import { useRouter } from "vue-router";
import {
  BookOpen, Building2, Headphones, LayoutDashboard,
  LogOut, MessagesSquare, RefreshCw, Users,
} from "lucide-vue-next";
import { APIError, get, messageForError, post } from "../api";
import { setSession, useSession } from "../composables/useSession";
import Brand from "../components/Brand.vue";
import Overview from "./Overview.vue";
import AccountsView from "./AccountsView.vue";
import KnowledgeView from "./KnowledgeView.vue";
import ConversationsView from "./ConversationsView.vue";
import TenantsView from "./TenantsView.vue";
import MembersView from "./MembersView.vue";
import CreateTenantDialog from "../components/CreateTenantDialog.vue";
import CreateAccountDialog from "../components/CreateAccountDialog.vue";
import EditAccountDialog from "../components/EditAccountDialog.vue";
import CreateKnowledgeDialog from "../components/CreateKnowledgeDialog.vue";
import InviteMemberDialog from "../components/InviteMemberDialog.vue";
import TenantCredentialsResult from "../components/TenantCredentialsResult.vue";
import InvitationResult from "../components/InvitationResult.vue";
import KnowledgeDetail from "./KnowledgeDetail.vue";
import EditKnowledgeDialog from "../components/EditKnowledgeDialog.vue";
import ChatView from "../components/ChatView.vue";

const { session } = useSession();
const router = useRouter();
const showToast = inject("showToast");

const NAV_SECTIONS = [
  {
    label: "运营中心",
    items: [
      { id: "overview", label: "总览", icon: LayoutDashboard },
      { id: "accounts", label: "客服账号", icon: Headphones },
      { id: "knowledge", label: "知识库", icon: BookOpen },
      { id: "conversations", label: "会话", icon: MessagesSquare },
    ],
  },
  {
    label: "组织管理",
    items: [
      { id: "tenants", label: "租户管理", icon: Building2 },
      { id: "members", label: "成员管理", icon: Users },
    ],
  },
];
const NAV_ITEMS = NAV_SECTIONS.flatMap((s) => s.items);
const TENANT_REQUIRED_VIEWS = new Set(["accounts", "knowledge", "conversations", "members"]);

const view = ref("overview");
const tenants = ref([]);
const platformRole = ref("");
const members = ref([]);
const accounts = ref([]);
const knowledgeBases = ref([]);
const conversations = ref([]);
const health = ref(null);
const loading = ref(true);
const selectingTenant = ref(false);
const refreshVersion = ref(0);

const createTenantOpen = ref(false);
const createAccountOpen = ref(false);
const editAccountOpen = ref(false);
const editingAccount = ref(null);
const createKnowledgeOpen = ref(false);
const inviteMemberOpen = ref(false);
const invitation = ref(null);
const tenantCredentials = ref(null);
const editKnowledgeOpen = ref(false);
const editingKnowledgeBase = ref(null);
const knowledgeBaseId = ref(null);
const chatAccount = ref(null);
const chatConversationId = ref("");
const chatReturnView = ref("accounts");

const activeTenant = computed(() =>
  tenants.value.find((t) => t.id === session.value?.activeTenantId) || null,
);
const selectableTenants = computed(() =>
  tenants.value.filter((t) => t.status === "active" && t.membershipStatus === "active"),
);
const canManageMembers = computed(() => activeTenant.value?.permissions?.includes("members:manage") || false);
const canManageAccounts = computed(() => activeTenant.value?.permissions?.includes("accounts:manage") || false);
const canManageKnowledge = computed(() => activeTenant.value?.permissions?.includes("knowledge:manage") || false);
const isPlatformAdmin = computed(() => !!platformRole.value);
const selectedKnowledgeBase = computed(() =>
  knowledgeBases.value.find((b) => b.id === knowledgeBaseId.value) || null,
);

function navigateToKnowledgeDetail(base) {
  knowledgeBaseId.value = base.id;
  view.value = "knowledgeDetail";
}

function backFromKnowledgeDetail() {
  knowledgeBaseId.value = null;
  navigate("knowledge");
}

function openAccountChat(account) {
  chatAccount.value = account;
  chatConversationId.value = "";
  chatReturnView.value = "accounts";
  view.value = "chat";
}

function openConversationChat(conversation) {
  const account = accounts.value.find((item) => item.id === conversation.accountId);
  if (!account) {
    showToast({ tone: "error", message: "未找到该会话所属的客服账号" });
    return;
  }
  chatAccount.value = account;
  chatConversationId.value = conversation.conversationId;
  chatReturnView.value = "conversations";
  view.value = "chat";
}

function closeChat() {
  chatAccount.value = null;
  chatConversationId.value = "";
  view.value = chatReturnView.value;
}

onMounted(() => {
  if (!session.value) { router.replace("/login"); return; }
  loadData();
});

watch(refreshVersion, () => loadData());

async function loadData() {
  loading.value = true;
  try {
    const [healthR, tenantR] = await Promise.all([get("/health"), get("/api/tenants")]);
    health.value = healthR;
    tenants.value = tenantR.tenants || [];
    platformRole.value = tenantR.platformRole || "";

    if (session.value?.activeTenantId) {
      const [memberR, acctR, kbR, convR] = await Promise.all([
        get("/api/members"),
        get("/api/accounts"),
        get("/api/knowledge/bases"),
        get("/api/conversations"),
      ]);
      members.value = memberR.members || [];
      accounts.value = acctR.accounts || [];
      knowledgeBases.value = kbR.bases || [];
      conversations.value = convR.conversations || [];
    } else {
      members.value = [];
      accounts.value = [];
      knowledgeBases.value = [];
      conversations.value = [];
    }
  } catch (e) {
    if (e instanceof APIError && e.status === 401) { setSession(null); router.replace("/login"); return; }
    showToast({ tone: "error", message: messageForError(e) });
  } finally {
    loading.value = false;
  }
}

async function refresh() {
  const s = await get("/api/auth/me");
  setSession(s);
  loadData();
}

async function selectTenant(tenantId) {
  if (selectingTenant.value || !tenantId) return;
  selectingTenant.value = true;
  try {
    await post("/api/auth/select-tenant", { tenantId }, session.value.csrfToken);
    const s = await get("/api/auth/me");
    setSession(s);
    view.value = "overview";
    showToast({ tone: "success", message: "工作区已切换" });
    loadData();
  } catch (e) {
    showToast({ tone: "error", message: messageForError(e) });
  } finally {
    selectingTenant.value = false;
  }
}

async function signOut() {
  try {
    await post("/api/auth/logout", {}, session.value.csrfToken);
  } catch (e) {
    showToast({ tone: "error", message: messageForError(e) });
    return;
  }
  setSession(null);
  router.replace("/login");
}

function navigate(v) {
  if (TENANT_REQUIRED_VIEWS.has(v) && !session.value?.activeTenantId) {
    showToast({ tone: "info", message: "请先选择一个租户工作区" });
    return;
  }
  view.value = v;
}
</script>

<template>
  <div class="dashboard-shell">
    <aside class="dashboard-sidebar">
      <div style="padding:20px 14px 0">
        <Brand />
      </div>
      <el-menu
        :default-active="view"
        background-color="#0d1f17"
        text-color="#b8c9bf"
        active-text-color="#fff"
        style="border-right:0;margin-top:12px"
        @select="navigate"
      >
        <template v-for="section in NAV_SECTIONS" :key="section.label">
          <el-menu-item-group :title="section.label">
            <el-menu-item v-for="item in section.items" :key="item.id" :index="item.id">
              <component :is="item.icon" :size="18" style="margin-right:8px" />
              <span>{{ item.label }}</span>
            </el-menu-item>
          </el-menu-item-group>
        </template>
      </el-menu>
    </aside>

    <div class="dashboard-main">
      <header class="topbar">
        <div class="topbar-left">
          <h2>
            <template v-if="view === 'knowledgeDetail'">
              <el-button text @click="backFromKnowledgeDetail" style="font-size:18px;padding:0">&larr; 知识库</el-button>
              <span style="color:#6b736d;margin:0 6px">/</span>
              {{ selectedKnowledgeBase?.name || '' }}
            </template>
            <template v-else>{{ NAV_ITEMS.find((i) => i.id === view)?.label }}</template>
          </h2>
          <p>
            {{ activeTenant ? activeTenant.name : isPlatformAdmin ? "平台管理空间" : "尚未选择租户工作区" }}
          </p>
        </div>
        <div class="topbar-right">
          <el-select
            v-if="selectableTenants.length"
            :model-value="session?.activeTenantId || ''"
            placeholder="选择工作区"
            size="small"
            style="width:180px"
            :disabled="selectingTenant"
            @change="selectTenant"
          >
            <el-option value="" disabled label="选择工作区" />
            <el-option v-for="t in selectableTenants" :key="t.id" :value="t.id" :label="t.name" />
          </el-select>
          <el-button :icon="RefreshCw" circle @click="refresh" />
          <span class="user-chip">
            <span class="user-chip-avatar">{{ (session?.user.displayName || session?.user.email || "").slice(0, 1).toUpperCase() }}</span>
            <span class="user-chip-name">{{ session?.user.displayName || session?.user.email }}</span>
          </span>
          <el-button type="danger" :icon="LogOut" circle plain aria-label="退出登录" title="退出登录" @click="signOut" />
        </div>
      </header>

      <div class="dashboard-content">
        <Overview
          v-if="!loading && view === 'overview'"
          :health="health" :active-tenant="activeTenant" :tenants="tenants"
          :accounts="accounts" :knowledge-bases="knowledgeBases" :conversations="conversations"
          :platform-role="platformRole" @navigate="navigate"
        />
        <AccountsView
          v-if="!loading && view === 'accounts'"
          :accounts="accounts" :can-manage="canManageAccounts" :csrf-token="session?.csrfToken"
          :knowledge-bases="knowledgeBases"
          @create="createAccountOpen = true"
          @edit="(acct) => { editingAccount = acct; editAccountOpen = true }"
          @chat="openAccountChat"
          @changed="loadData()"
        />
        <KnowledgeView
          v-if="!loading && view === 'knowledge'"
          :bases="knowledgeBases" :can-manage="canManageKnowledge"
          :csrf-token="session?.csrfToken"
          @create="createKnowledgeOpen = true"
          @edit="(b) => { editingKnowledgeBase = b; editKnowledgeOpen = true }"
          @detail="navigateToKnowledgeDetail"
          @changed="loadData()"
        />
        <KnowledgeDetail
          v-if="!loading && view === 'knowledgeDetail' && selectedKnowledgeBase"
          :key="knowledgeBaseId"
          :base="selectedKnowledgeBase"
          :can-manage="canManageKnowledge"
          :csrf-token="session?.csrfToken"
          @back="backFromKnowledgeDetail"
          @base-updated="() => { editingKnowledgeBase = selectedKnowledgeBase; editKnowledgeOpen = true; }"
          @articles-changed="loadData()"
        />
        <ConversationsView v-if="!loading && view === 'conversations'" :conversations="conversations" :accounts="accounts" :can-manage="canManageAccounts" :csrf-token="session?.csrfToken" @chat="openConversationChat" @changed="loadData()" />
        <ChatView
          v-if="!loading && view === 'chat' && chatAccount"
          :key="`${chatAccount.id}:${chatConversationId}`"
          :account="chatAccount"
          :csrf-token="session?.csrfToken"
          :initial-conversation-id="chatConversationId"
          @back="closeChat"
        />
        <TenantsView
          v-if="!loading && view === 'tenants'" :tenants="tenants" :platform-role="platformRole"
          :active-tenant-id="session?.activeTenantId" :csrf-token="session?.csrfToken"
          @select="selectTenant" @create="createTenantOpen = true" @changed="loadData()"
        />
        <MembersView
          v-if="!loading && view === 'members'" :members="members" :can-manage="canManageMembers"
          :csrf-token="session?.csrfToken" @invite="inviteMemberOpen = true" @changed="loadData()"
        />
      </div>
    </div>

    <CreateTenantDialog v-if="createTenantOpen" :csrf-token="session?.csrfToken"
      @close="createTenantOpen = false" @created="(c) => { createTenantOpen = false; tenantCredentials = c; loadData(); }" />
    <CreateAccountDialog v-if="createAccountOpen" :csrf-token="session?.csrfToken"
      @close="createAccountOpen = false" @created="() => { createAccountOpen = false; loadData(); }" />
    <EditAccountDialog
      v-if="editAccountOpen && editingAccount"
      :account="editingAccount"
      :csrf-token="session?.csrfToken"
      @close="editAccountOpen = false; editingAccount = null"
      @updated="() => { editAccountOpen = false; editingAccount = null; loadData(); }"
    />
    <CreateKnowledgeDialog v-if="createKnowledgeOpen" :csrf-token="session?.csrfToken"
      @close="createKnowledgeOpen = false" @created="() => { createKnowledgeOpen = false; loadData(); }" />
    <InviteMemberDialog v-if="inviteMemberOpen" :csrf-token="session?.csrfToken"
      @close="inviteMemberOpen = false" @invited="(i) => { inviteMemberOpen = false; invitation = i.invitation; }" />
    <TenantCredentialsResult v-if="tenantCredentials" :created="tenantCredentials" @close="tenantCredentials = null" />
    <InvitationResult v-if="invitation" :invitation="invitation" @close="invitation = null" />
    <EditKnowledgeDialog
      v-if="editKnowledgeOpen && editingKnowledgeBase"
      :base="editingKnowledgeBase"
      :csrf-token="session?.csrfToken"
      @close="editKnowledgeOpen = false; editingKnowledgeBase = null"
      @updated="() => { editKnowledgeOpen = false; editingKnowledgeBase = null; loadData(); }"
      @deleted="() => { editKnowledgeOpen = false; editingKnowledgeBase = null; backFromKnowledgeDetail(); loadData(); }"
    />
  </div>
</template>
