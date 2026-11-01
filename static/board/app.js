const { createApp } = Vue;

function today() {
  const now = new Date();
  const month = String(now.getMonth() + 1).padStart(2, "0");
  const day = String(now.getDate()).padStart(2, "0");
  return `${now.getFullYear()}-${month}-${day}`;
}

createApp({
  data() {
    return {
      board: { columns: [], cards: [], events: [], members: [], public: true },
      boards: [],
      user: null,
      token: localStorage.getItem("board-token") || "",
      title: "", description: "", assignee: "", due: "", tag: "",
      query: "", tagFilter: "", open: "", drag: "",
      freshTitle: "", template: "dev", inviteEmail: "", columnTitle: "",
      authName: "", authEmail: "", authPass: "",
      error: "", socketBoard: "", tries: 0,
      ready: false, live: false, composer: "", pop: "", arm: "",
      overCol: "", overCard: "", didDrag: false, errorStamp: 0,
    };
  },
  computed: {
    mine() {
      return !!(this.user && this.board.ownerId === this.user.id);
    },
    roleLabel() {
      if (!this.user) return "";
      if (this.board.public) return "общая";
      if (this.mine) return "владелец";
      return "участник";
    },
    people() {
      const names = [];
      const push = (name) => {
        if (name && name !== "Общая" && !names.includes(name)) names.push(name);
      };
      push(this.board.owner);
      (this.board.members || []).forEach((item) => push(item.name));
      (this.board.cards || []).forEach((card) => push(card.assignee));
      return names;
    },
    faces() {
      return this.people.slice(0, 6);
    },
    extraFaces() {
      return Math.max(0, this.people.length - 6);
    },
    tags() {
      const set = new Set();
      (this.board.cards || []).forEach((card) => (card.tags || []).forEach((tag) => set.add(tag)));
      return [...set];
    },
    filtering() {
      return !!(this.query.trim() || this.tagFilter);
    },
    matchCount() {
      return (this.board.columns || []).reduce((sum, column) => sum + this.shown(column.id).length, 0);
    },
    history() {
      return (this.board.events || []).slice().reverse();
    },
    active() {
      return (this.board.cards || []).find((card) => card.id === this.open) || null;
    },
  },
  mounted() {
    this.keyHandler = (event) => this.onKey(event);
    window.addEventListener("keydown", this.keyHandler);
    this.load("demo").then(() => this.watch());
  },
  beforeUnmount() {
    window.removeEventListener("keydown", this.keyHandler);
    if (this.ws) this.ws.close();
  },
  methods: {
    onKey(event) {
      if (event.key === "Escape") {
        if (this.composer) { this.composer = ""; return; }
        if (this.open) { this.open = ""; return; }
        this.pop = "";
        this.arm = "";
        return;
      }
      if (event.key === "/" && !this.typing(event)) {
        event.preventDefault();
        if (this.$refs.search) this.$refs.search.focus();
      }
    },
    typing(event) {
      const tag = event.target && event.target.tagName;
      return tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT";
    },
    cardsIn(column) {
      return (this.board.cards || []).filter((card) => card.column === column).sort((a, b) => a.pos - b.pos);
    },
    shown(column) {
      const needle = this.query.trim().toLowerCase();
      return this.cardsIn(column).filter((card) => {
        if (this.tagFilter && !(card.tags || []).includes(this.tagFilter)) return false;
        if (!needle) return true;
        return `${card.title} ${card.description || ""}`.toLowerCase().includes(needle);
      });
    },
    countLabel(column) {
      const all = this.cardsIn(column.id).length;
      const view = this.shown(column.id).length;
      if (this.filtering && view !== all) return `${view}/${all}`;
      return String(all);
    },
    late(card) {
      const column = (this.board.columns || []).find((item) => item.id === card.column);
      if (!card.due || (column && column.title === "Готово")) return false;
      return card.due < today();
    },
    sameDay(card) {
      return !!card.due && card.due === today();
    },
    dueLabel(card) {
      if (!card.due) return "";
      if (this.late(card)) return "просрочено";
      if (this.sameDay(card)) return "сегодня";
      const [year, month, day] = card.due.split("-");
      if (Number(year) !== new Date().getFullYear()) return `${day}.${month}.${year}`;
      return `${day}.${month}`;
    },
    tone(column) {
      const title = (column.title || "").toLowerCase();
      if (title.includes("готово")) return "done";
      if (title.includes("провер")) return "review";
      if (title.includes("работ") || title.includes("дела")) return "work";
      return "";
    },
    columnTitleOf(card) {
      const column = (this.board.columns || []).find((item) => item.id === card.column);
      return column ? column.title : "";
    },
    initials(name) {
      return String(name || "?")
        .trim()
        .split(/\s+/)
        .slice(0, 2)
        .map((part) => part[0] || "")
        .join("")
        .toUpperCase();
    },
    paint(name) {
      let hash = 0;
      for (const ch of String(name || "?")) hash = (hash * 33 + ch.charCodeAt(0)) >>> 0;
      return { background: `hsl(${hash % 360} 38% 36%)` };
    },
    checksDone(card) {
      return (card.checks || []).filter((item) => item.done).length;
    },
    ratio(card) {
      const all = (card.checks || []).length;
      if (!all) return "0%";
      return `${Math.round((100 * this.checksDone(card)) / all)}%`;
    },
    holdError() {
      const stamp = Date.now();
      this.errorStamp = stamp;
      setTimeout(() => { if (this.errorStamp === stamp) this.error = ""; }, 4200);
    },
    async request(path, method, body) {
      const headers = {};
      if (this.token) headers["X-Board-Token"] = this.token;
      if (body) headers["Content-Type"] = "application/json";
      try {
        const response = await fetch(path, { method, headers, body: body ? JSON.stringify(body) : undefined });
        const data = await response.json().catch(() => ({}));
        if (!response.ok) {
          this.error = data.message || "Не вышло";
          this.holdError();
          return null;
        }
        this.error = "";
        return data;
      } catch (err) {
        this.error = "Нет связи";
        this.holdError();
        return null;
      }
    },
    apply(data) {
      if (!data || !data.board) return;
      const drafts = {};
      (this.board.cards || []).forEach((card) => { drafts[card.id] = card; });
      data.board.cards.forEach((card) => {
        const prev = drafts[card.id] || {};
        card.draft = prev.draft || "";
        card.checkDraft = prev.checkDraft || "";
        card.linkName = prev.linkName || "";
        card.linkUrl = prev.linkUrl || "";
        card.tagDraft = prev.tagDraft || "";
      });
      this.board = data.board;
      this.boards = data.boards || [];
      if (data.user) this.user = data.user;
      this.ready = true;
    },
    async load(id) {
      const board = id || this.board.id || "demo";
      const data = await this.request(`api/state?board=${encodeURIComponent(board)}`, "GET");
      if (data) this.apply(data);
      this.ready = true;
    },
    watch() {
      if (!this.board.id) return;
      const proto = location.protocol === "https:" ? "wss" : "ws";
      const socket = new WebSocket(`${proto}://${location.host}/board/ws`);
      const prev = this.ws;
      this.ws = socket;
      this.socketBoard = this.board.id;
      if (prev) prev.close();
      socket.onopen = () => {
        this.tries = 0;
        this.live = true;
        socket.send(JSON.stringify({ board: this.board.id, token: this.token }));
      };
      socket.onmessage = () => {
        if (socket === this.ws) this.load(this.board.id);
      };
      socket.onclose = () => {
        if (socket !== this.ws) return;
        this.live = false;
        if (this.tries >= 4) return;
        this.tries += 1;
        setTimeout(() => { if (this.socketBoard === this.board.id) this.watch(); }, 1500);
      };
    },
    toggle(name) {
      this.arm = "";
      this.pop = this.pop === name ? "" : name;
    },
    clearFilter() {
      this.query = "";
      this.tagFilter = "";
    },
    openComposer(id) {
      this.composer = id;
      this.pop = "";
      this.$nextTick(() => {
        const ref = this.$refs.draftTitle;
        const node = Array.isArray(ref) ? ref[0] : ref;
        if (node && node.focus) node.focus();
      });
    },
    openCard(card) {
      if (this.didDrag) return;
      this.open = card.id;
      this.pop = "";
      this.arm = "";
    },
    async pick(id) {
      this.open = "";
      this.composer = "";
      this.pop = "";
      await this.load(id);
      this.watch();
    },
    async add() {
      const data = await this.request("api/card", "POST", {
        board: this.board.id, column: this.composer, title: this.title, description: this.description,
        assignee: this.assignee, due: this.due, tag: this.tag,
      });
      this.apply(data);
      if (data) {
        this.title = "";
        this.description = "";
        this.assignee = "";
        this.due = "";
        this.tag = "";
        this.composer = "";
      }
    },
    async save(card) {
      const data = await this.request("api/card/update", "POST", {
        board: this.board.id, id: card.id, title: card.title, description: card.description,
        assignee: card.assignee, due: card.due, tags: (card.tags || []).join(", "),
      });
      if (!data) {
        await this.load(this.board.id);
        return;
      }
      this.apply(data);
    },
    async move(card, column, before) {
      const data = await this.request("api/move", "POST", { board: this.board.id, id: card.id || card, column, before: before || "" });
      this.apply(data);
    },
    nudge(card, dir) {
      const list = this.cardsIn(card.column);
      const index = list.findIndex((item) => item.id === card.id);
      if (dir < 0 && index > 0) this.move(card, card.column, list[index - 1].id);
      if (dir > 0 && index >= 0 && index < list.length - 1) {
        const next = list[index + 2];
        this.move(card, card.column, next ? next.id : "");
      }
    },
    dragStart(card, event) {
      this.drag = card.id;
      this.didDrag = false;
      event.dataTransfer.effectAllowed = "move";
      event.dataTransfer.setData("text/plain", card.id);
    },
    hoverColumn(column) {
      this.didDrag = true;
      this.overCol = column.id;
    },
    hoverCard(card) {
      this.didDrag = true;
      this.overCol = card.column;
      this.overCard = card.id;
    },
    dragEnd() {
      this.drag = "";
      this.overCol = "";
      this.overCard = "";
      setTimeout(() => { this.didDrag = false; }, 80);
    },
    dropColumn(column) {
      if (this.drag) this.move({ id: this.drag }, column.id);
      this.drag = "";
      this.overCol = "";
    },
    dropCard(card) {
      if (this.drag && this.drag !== card.id) this.move({ id: this.drag }, card.column, card.id);
      this.drag = "";
      this.overCard = "";
    },
    async comment(card) {
      const text = (card.draft || "").trim();
      if (!text) return;
      card.draft = "";
      const data = await this.request("api/comment", "POST", { board: this.board.id, id: card.id, text });
      if (!data) card.draft = text;
      this.apply(data);
    },
    async addCheck(card) {
      const text = (card.checkDraft || "").trim();
      if (!text) return;
      card.checkDraft = "";
      const data = await this.request("api/check", "POST", { board: this.board.id, id: card.id, text });
      if (!data) card.checkDraft = text;
      this.apply(data);
    },
    async toggle(card, item) {
      const data = await this.request("api/check/toggle", "POST", { board: this.board.id, id: card.id, check: item.id });
      this.apply(data);
    },
    async pushTag(card) {
      const tag = (card.tagDraft || "").trim();
      if (!tag) return;
      const tags = [...(card.tags || [])];
      if (!tags.includes(tag)) {
        if (tags.length >= 4) {
          this.error = "Меток уже четыре";
          this.holdError();
          return;
        }
        tags.push(tag);
      }
      card.tags = tags;
      card.tagDraft = "";
      await this.save(card);
    },
    async removeTag(card, tag) {
      card.tags = (card.tags || []).filter((item) => item !== tag);
      await this.save(card);
    },
    async addLink(card) {
      const name = card.linkName;
      const url = card.linkUrl;
      if (!(name || "").trim() || !(url || "").trim()) return;
      card.linkName = "";
      card.linkUrl = "";
      const data = await this.request("api/link", "POST", { board: this.board.id, id: card.id, name, url });
      if (!data) {
        card.linkName = name;
        card.linkUrl = url;
      }
      this.apply(data);
    },
    async dropCardOut(card) {
      if (this.arm !== card.id) { this.arm = card.id; return; }
      const data = await this.request("api/card/delete", "POST", { board: this.board.id, id: card.id });
      if (data && this.open === card.id) this.open = "";
      this.arm = "";
      this.apply(data);
    },
    async renameBoard(title) {
      const data = await this.request("api/rename", "POST", { board: this.board.id, title });
      if (!data) { await this.load(this.board.id); return; }
      this.apply(data);
    },
    async renameColumn(column, title) {
      const data = await this.request("api/column/rename", "POST", { board: this.board.id, id: column.id, title });
      if (!data) { await this.load(this.board.id); return; }
      this.apply(data);
    },
    async addColumn() {
      const data = await this.request("api/column", "POST", { board: this.board.id, title: this.columnTitle });
      this.apply(data);
      if (data) this.columnTitle = "";
    },
    async create() {
      const data = await this.request("api/create", "POST", { title: this.freshTitle, template: this.template });
      this.apply(data);
      if (data) {
        this.freshTitle = "";
        this.pop = "";
        this.open = "";
        this.watch();
      }
    },
    async invite() {
      const data = await this.request("api/invite", "POST", { board: this.board.id, email: this.inviteEmail });
      this.apply(data);
      if (data) this.inviteEmail = "";
    },
    async removeBoard() {
      if (this.arm !== "board") { this.arm = "board"; return; }
      const data = await this.request("api/delete", "POST", { board: this.board.id });
      this.arm = "";
      this.pop = "";
      this.open = "";
      this.apply(data);
      if (data) this.watch();
    },
    async reset() {
      if (this.arm !== "reset") { this.arm = "reset"; return; }
      const data = await this.request("api/reset", "POST", {});
      this.arm = "";
      this.open = "";
      this.apply(data);
    },
    keep(data) {
      if (!data || !data.token) return;
      this.token = data.token;
      this.user = data.user;
      localStorage.setItem("board-token", data.token);
    },
    async login() {
      const data = await this.request("api/login", "POST", { email: this.authEmail, password: this.authPass });
      this.keep(data);
      if (data) {
        this.pop = "";
        this.authPass = "";
        await this.load(this.board.id || "demo");
        this.watch();
      }
    },
    async register() {
      const data = await this.request("api/register", "POST", { name: this.authName, email: this.authEmail, password: this.authPass });
      this.keep(data);
      if (data) {
        this.authPass = "";
        this.pop = "";
        await this.load("demo");
        this.watch();
      }
    },
    async demo(kind) {
      const data = await this.request("api/demo", "POST", { kind });
      this.keep(data);
      if (!data) return;
      this.pop = "";
      this.open = "";
      const state = await this.request("api/state?board=sprint", "GET");
      this.apply(state);
      this.watch();
    },
    async logout() {
      await this.request("api/logout", "POST", {});
      this.token = "";
      this.user = null;
      this.pop = "";
      this.open = "";
      localStorage.removeItem("board-token");
      await this.load("demo");
      this.watch();
    },
  },
}).mount("#app");
