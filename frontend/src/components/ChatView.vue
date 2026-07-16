<script setup>
import { computed, inject, nextTick, onUnmounted, ref, watch } from "vue";
import {
  ArrowLeft,
  BookOpen,
  Circle,
  Info,
  MessageSquareText,
  Search,
  SendHorizontal,
  ShieldCheck,
  SlidersHorizontal,
  UserRound,
} from "lucide-vue-next";
import { get, messageForError, post } from "../api";
import { formatDate } from "../utils";

const props = defineProps({
  account: { type: Object, required: true },
  knowledgeBases: { type: Array, default: () => [] },
  csrfToken: { type: String, default: "" },
  initialConversationId: { type: String, default: "" },
});
const emit = defineEmits(["back"]);
const showToast = inject("showToast");

const conversations = ref([]);
const selectedConv = ref(null);
const messages = ref([]);
const searchText = ref("");
const loadingConvs = ref(false);
const loadingMsgs = ref(false);
const replyText = ref("");
const sending = ref(false);
const chatBody = ref(null);
const searchInput = ref(null);
const inspector = ref(null);
let pollTimer = null;
let messageRequestVersion = 0;
let polling = false;

const filteredConversations = computed(() => {
  const query = searchText.value.trim().toLowerCase();
  if (!query) return conversations.value;
  return conversations.value.filter((conv) =>
    [conv.customerName, conv.lastMessage, conv.conversationId]
      .filter(Boolean)
      .some((value) => String(value).toLowerCase().includes(query)),
  );
});

const accountStatus = computed(() => {
  if (props.account.status === "connected") return { label: "已连接", tone: "success" };
  if (props.account.status === "disabled") return { label: "已停用", tone: "warning" };
  return { label: "待连接", tone: "info" };
});

const accountKnowledgeNames = computed(() => {
  const map = new Map((props.knowledgeBases || []).map((base) => [base.id, base.name]));
  return (props.account.kbId || []).map((id) => map.get(id) || id.slice(0, 8));
});

const knowledgePreview = computed(() => accountKnowledgeNames.value.slice(0, 4));
const knowledgeRemaining = computed(() => Math.max(accountKnowledgeNames.value.length - knowledgePreview.value.length, 0));
const dailyLimitText = computed(() =>
  props.account.dailyLimit ? `${props.account.dailyReplies || 0} / ${props.account.dailyLimit}` : `${props.account.dailyReplies || 0} / 不限`,
);
const dailyProgress = computed(() => {
  if (!props.account.dailyLimit) return 0;
  return Math.min(100, Math.round(((props.account.dailyReplies || 0) / props.account.dailyLimit) * 100));
});
const hasSendableTarget = computed(() => /^\+\d{7,15}$/.test(selectedConv.value?.conversationId || ""));
const canSend = computed(() => Boolean(!sending.value && selectedConv.value && hasSendableTarget.value && replyText.value.trim()));
const recipientHint = computed(() =>
  hasSendableTarget.value ? "消息将通过当前 WhatsApp 账号发送" : "该会话没有可投递的 E.164 手机号，暂不可直接发送",
);

function initials(value) {
  return String(value || "客").trim().slice(0, 1).toUpperCase();
}

function messageReference(value) {
  if (!value || value === "[]") return "";
  try {
    const references = JSON.parse(value);
    return Array.isArray(references) && references.length ? `参考 ${references.length} 个知识片段` : "";
  } catch {
    return "已参考知识库";
  }
}

async function loadConversations() {
  loadingConvs.value = true;
  try {
    const resp = await get(`/api/conversations?accountId=${props.account.id}`);
    conversations.value = resp.conversations || [];
    const requested = conversations.value.find((conv) => conv.conversationId === props.initialConversationId);
    const firstConversation = requested || (!props.initialConversationId ? conversations.value[0] : null);
    if (firstConversation) await loadMessages(firstConversation);
  } catch (error) {
    showToast({ tone: "error", message: messageForError(error) });
  } finally {
    loadingConvs.value = false;
  }
}

async function loadMessages(conv) {
  const requestVersion = ++messageRequestVersion;
  selectedConv.value = conv;
  loadingMsgs.value = true;
  try {
    const limit = props.account.replyLimit || 30;
    const resp = await get(`/api/conversations/${conv.conversationId}/messages?accountId=${encodeURIComponent(props.account.id)}&limit=${limit}`);
    if (requestVersion !== messageRequestVersion) return;
    messages.value = (resp.messages || []).reverse();
  } catch (error) {
    if (requestVersion !== messageRequestVersion) return;
    messages.value = [];
    showToast({ tone: "error", message: messageForError(error) });
  } finally {
    if (requestVersion === messageRequestVersion) {
      loadingMsgs.value = false;
      await nextTick();
      scrollToBottom();
    }
  }
}

function scrollToBottom() {
  if (chatBody.value) chatBody.value.scrollTop = chatBody.value.scrollHeight;
}

function focusConversationSearch() {
  searchInput.value?.focus?.();
}

function showConversationInfo() {
  inspector.value?.scrollIntoView?.({ behavior: "smooth", block: "start" });
}

async function sendReply() {
	const text = replyText.value.trim();
	if (sending.value || !text || !selectedConv.value || !hasSendableTarget.value) return;
  sending.value = true;
  try {
    await post(
      `/api/conversations/${encodeURIComponent(selectedConv.value.conversationId)}/send`,
      { accountId: props.account.id, customerName: selectedConv.value.customerName, content: text },
      props.csrfToken,
    );
    replyText.value = "";
    await loadMessages(selectedConv.value);
  } catch (error) {
    showToast({ tone: "error", message: messageForError(error) });
  } finally {
    sending.value = false;
  }
}

function handleKeydown(event) {
  if (event.key === "Enter" && !event.shiftKey) {
    event.preventDefault();
    sendReply();
  }
}

function startPolling(conv) {
  clearInterval(pollTimer);
  pollTimer = setInterval(async () => {
    if (polling || selectedConv.value?.conversationId !== conv.conversationId) return;
    polling = true;
    try {
      const limit = props.account.replyLimit || 30;
      const resp = await get(`/api/conversations/${conv.conversationId}/messages?accountId=${encodeURIComponent(props.account.id)}&limit=${limit}`);
      const latest = (resp.messages || []).reverse();
      if (latest.length !== messages.value.length || latest.at(-1)?.id !== messages.value.at(-1)?.id) {
        messages.value = latest;
        await nextTick();
        scrollToBottom();
      }
    } catch {
      // Polling failures should not interrupt manual replies.
    } finally {
      polling = false;
    }
  }, 5000);
}

watch(selectedConv, (conv) => {
  if (conv) startPolling(conv);
  else clearInterval(pollTimer);
});

onUnmounted(() => clearInterval(pollTimer));

loadConversations();
</script>

<template>
  <div class="chat-workspace">
    <aside class="chat-rail">
      <header class="chat-rail-header">
        <div class="chat-account-heading">
          <el-button class="chat-icon-button" text :icon="ArrowLeft" aria-label="返回客服账号" title="返回客服账号" @click="emit('back')" />
          <div>
            <h3>{{ account.name }}</h3>
            <p>{{ conversations.length }} 个会话</p>
          </div>
        </div>
        <el-button class="chat-icon-button" text :icon="SlidersHorizontal" aria-label="会话筛选" title="会话筛选" @click="focusConversationSearch" />
      </header>
      <div class="chat-search-box">
        <el-input ref="searchInput" v-model="searchText" clearable placeholder="搜索会话">
          <template #prefix><Search :size="16" /></template>
        </el-input>
      </div>
      <div v-loading="loadingConvs" class="chat-thread-list">
        <button
          v-for="conv in filteredConversations"
          :key="conv.conversationId"
          type="button"
          class="chat-thread"
          :class="{ 'is-selected': selectedConv?.conversationId === conv.conversationId }"
          @click="loadMessages(conv)"
        >
          <span class="chat-avatar">{{ initials(conv.customerName) }}</span>
          <span class="chat-thread-content">
            <span class="chat-thread-topline">
              <strong>{{ conv.customerName || "未命名客户" }}</strong>
              <time>{{ conv.lastMessageAt ? formatDate(conv.lastMessageAt) : "" }}</time>
            </span>
            <span class="chat-thread-preview">{{ conv.lastMessage || "暂无消息" }}</span>
            <span class="chat-thread-meta">
              <span>{{ conv.messageCount || 0 }} 条消息</span>
              <span v-if="/^\+\d{7,15}$/.test(conv.conversationId)">WhatsApp</span>
            </span>
          </span>
        </button>
        <div v-if="!filteredConversations.length && !loadingConvs" class="chat-empty-rail">
          <MessageSquareText :size="22" />
          <strong>{{ searchText ? "没有匹配会话" : "暂无会话" }}</strong>
          <span>{{ searchText ? "换个关键词试试" : "接入 WhatsApp 后自动显示" }}</span>
        </div>
      </div>
    </aside>

    <main class="chat-main-panel">
      <header v-if="selectedConv" class="chat-main-header">
        <div class="chat-main-identity">
          <span class="chat-avatar is-large">{{ initials(selectedConv.customerName) }}</span>
          <div>
            <h2>{{ selectedConv.customerName || "未命名客户" }}</h2>
            <p><Circle :size="8" fill="currentColor" /> {{ hasSendableTarget ? selectedConv.conversationId : "历史会话" }}</p>
          </div>
        </div>
        <div class="chat-main-actions">
          <span class="chat-message-count">{{ selectedConv.messageCount || messages.length }} 条消息</span>
          <el-button class="chat-icon-button" text :icon="Info" aria-label="查看会话信息" title="查看会话信息" @click="showConversationInfo" />
        </div>
      </header>
      <div v-else class="chat-main-header is-empty">
        <div class="chat-main-identity"><span class="chat-avatar is-large">?</span><div><h2>选择一个会话</h2><p>从左侧列表打开客户对话</p></div></div>
      </div>

      <div ref="chatBody" class="chat-message-stream">
        <div v-if="!selectedConv" class="chat-empty-main">
          <span class="chat-empty-icon"><MessageSquareText :size="24" /></span>
          <strong>开始处理客户会话</strong>
          <span>选择左侧客户后，这里会显示完整消息记录。</span>
        </div>
        <div v-else-if="loadingMsgs" class="chat-empty-main"><el-icon class="is-loading" :size="24" /></div>
        <div v-else-if="!messages.length" class="chat-empty-main">
          <span class="chat-empty-icon"><MessageSquareText :size="24" /></span>
          <strong>暂无消息记录</strong>
          <span>发送第一条人工回复，开始处理这个客户。</span>
        </div>
        <template v-else>
          <div class="chat-date-divider"><span>消息记录</span></div>
          <article v-for="message in messages" :key="message.id" class="chat-message-row" :class="{ 'is-customer': message.role === 'customer' }">
            <div class="chat-message-bubble">
              <div class="chat-message-meta">
                <strong>{{ message.role === "customer" ? message.customerName || "客户" : "客服" }}</strong>
                <time>{{ formatDate(message.createdAt) }}</time>
              </div>
              <p>{{ message.content }}</p>
              <div v-if="messageReference(message.knowledgeIds)" class="chat-message-reference"><BookOpen :size="13" /> {{ messageReference(message.knowledgeIds) }}</div>
            </div>
          </article>
        </template>
      </div>

      <footer v-if="selectedConv" class="chat-composer">
        <div class="chat-composer-topline">
          <span :class="{ 'is-warning': !hasSendableTarget }"><ShieldCheck :size="14" /> {{ recipientHint }}</span>
          <span>Enter 发送 · Shift + Enter 换行</span>
        </div>
        <div class="chat-composer-row">
          <el-input
            v-model="replyText"
            class="chat-composer-input"
            type="textarea"
            :rows="2"
            :disabled="!hasSendableTarget"
            placeholder="输入人工回复..."
            @keydown="handleKeydown"
          />
          <el-button type="primary" class="chat-send-button" :icon="SendHorizontal" :loading="sending" :disabled="!canSend" @click="sendReply">发送</el-button>
        </div>
      </footer>
    </main>

    <aside ref="inspector" class="chat-inspector">
      <div class="chat-inspector-header"><h3>会话上下文</h3><Info :size="16" /></div>
      <section class="chat-inspector-section">
        <div class="chat-inspector-section-title"><span>客服账号</span><span class="chat-status" :class="`is-${accountStatus.tone}`"><Circle :size="8" fill="currentColor" /> {{ accountStatus.label }}</span></div>
        <div class="chat-account-card">
          <span class="chat-account-mark"><MessageSquareText :size="18" /></span>
          <div><strong>{{ account.name }}</strong><span>{{ account.accountKey }}</span></div>
        </div>
        <div class="chat-capacity"><div><span>今日回复</span><strong>{{ dailyLimitText }}</strong></div><div class="chat-capacity-track"><span :style="{ width: `${dailyProgress}%` }" /></div></div>
      </section>
      <section class="chat-inspector-section">
        <div class="chat-inspector-section-title"><span>知识库范围</span><BookOpen :size="15" /></div>
        <div class="chat-scope-count"><strong>{{ accountKnowledgeNames.length }}</strong><span>个知识库已绑定</span></div>
        <ul v-if="knowledgePreview.length" class="chat-scope-list">
          <li v-for="name in knowledgePreview" :key="name">{{ name }}</li>
          <li v-if="knowledgeRemaining" class="is-muted">另 {{ knowledgeRemaining }} 个知识库</li>
        </ul>
        <p v-else class="chat-inspector-muted">当前账号未绑定知识库</p>
      </section>
      <section class="chat-inspector-section">
        <div class="chat-inspector-section-title"><span>客户信息</span><UserRound :size="15" /></div>
        <dl class="chat-detail-list">
          <div><dt>客户名称</dt><dd>{{ selectedConv?.customerName || "未选择" }}</dd></div>
          <div><dt>会话状态</dt><dd>{{ selectedConv ? "处理中" : "等待选择" }}</dd></div>
          <div><dt>消息数量</dt><dd>{{ selectedConv?.messageCount || messages.length || 0 }} 条</dd></div>
          <div><dt>最后活跃</dt><dd>{{ selectedConv?.lastMessageAt ? formatDate(selectedConv.lastMessageAt) : "-" }}</dd></div>
        </dl>
      </section>
      <section class="chat-inspector-note"><ShieldCheck :size="16" /><span>人工回复会经过账号容量检查，并记录到当前会话。</span></section>
    </aside>
  </div>
</template>
