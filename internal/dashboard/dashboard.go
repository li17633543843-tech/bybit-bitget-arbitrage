package dashboard

import (
	"encoding/json"
	"html/template"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/li17633543843-tech/bybit-bitget-arbitrage/internal/orderbook"
	"github.com/li17633543843-tech/bybit-bitget-arbitrage/internal/paper"
	"github.com/li17633543843-tech/bybit-bitget-arbitrage/internal/strategy"
)

type Opportunity struct {
	strategy.DepthOpportunity
	UpdatedAt time.Time `json:"updated_at"`
}

type BookStatus struct {
	Exchange  string    `json:"exchange"`
	Symbol    string    `json:"symbol"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Payload struct {
	StartedAt     time.Time     `json:"started_at"`
	ServerTime    time.Time     `json:"server_time"`
	Snapshot      paper.Snapshot `json:"snapshot"`
	Opportunities []Opportunity `json:"opportunities"`
	Books         []BookStatus  `json:"books"`
	RecentEvents  []paper.Event `json:"recent_events"`
}

type State struct {
	mu            sync.RWMutex
	startedAt     time.Time
	snapshot      paper.Snapshot
	opportunities map[string]Opportunity
	books         map[string]BookStatus
	recentEvents  []paper.Event
}

func New() *State {
	return &State{startedAt: time.Now(), opportunities: make(map[string]Opportunity), books: make(map[string]BookStatus)}
}

func (s *State) RecordBook(book orderbook.Book) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := book.Exchange + ":" + book.Symbol
	s.books[key] = BookStatus{Exchange: book.Exchange, Symbol: book.Symbol, UpdatedAt: book.UpdatedAt}
}

func (s *State) RecordOpportunity(now time.Time, value strategy.DepthOpportunity) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := value.Symbol + ":" + value.BuyExchange + ":" + value.SellExchange
	s.opportunities[key] = Opportunity{DepthOpportunity: value, UpdatedAt: now}
}

func (s *State) RecordPaper(snapshot paper.Snapshot, events []paper.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshot = snapshot
	s.recentEvents = append(s.recentEvents, events...)
	if len(s.recentEvents) > 100 {
		s.recentEvents = append([]paper.Event(nil), s.recentEvents[len(s.recentEvents)-100:]...)
	}
}

func (s *State) payload(now time.Time) Payload {
	s.mu.RLock()
	defer s.mu.RUnlock()
	opportunities := make([]Opportunity, 0, len(s.opportunities))
	for _, value := range s.opportunities {
		if now.Sub(value.UpdatedAt) > 30*time.Second {
			continue
		}
		opportunities = append(opportunities, value)
	}
	sort.Slice(opportunities, func(i, j int) bool { return opportunities[i].NetEdgeBps > opportunities[j].NetEdgeBps })
	books := make([]BookStatus, 0, len(s.books))
	for _, value := range s.books {
		books = append(books, value)
	}
	return Payload{StartedAt: s.startedAt, ServerTime: now, Snapshot: s.snapshot, Opportunities: opportunities, Books: books, RecentEvents: append([]paper.Event(nil), s.recentEvents...)}
}

func (s *State) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(s.payload(time.Now()))
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok\n")) })
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = page.Execute(w, nil)
	})
	return mux
}

var page = template.Must(template.New("dashboard").Parse(`<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>跨所价差监控</title><style>
:root{color-scheme:dark;font-family:Inter,system-ui,sans-serif;background:#07111f;color:#e5edf8}body{margin:0;padding:24px;max-width:1400px;margin:auto}h1{margin:0 0 6px}.muted{color:#91a4bc}.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:12px;margin:20px 0}.card{background:#101d2f;border:1px solid #21334b;border-radius:12px;padding:16px}.value{font-size:26px;font-weight:700;margin-top:6px}.good{color:#4ade80}.bad{color:#fb7185}table{width:100%;border-collapse:collapse;background:#101d2f;border-radius:12px;overflow:hidden;margin-bottom:20px}th,td{text-align:left;padding:10px;border-bottom:1px solid #21334b;font-variant-numeric:tabular-nums}th{color:#91a4bc}h2{margin-top:26px}@media(max-width:700px){body{padding:12px;overflow-x:auto}th,td{padding:8px;font-size:13px}}
</style></head><body><h1>Bybit × Bitget 套利监控</h1><div class="muted" id="status">连接中…</div><div class="grid" id="cards"></div>
<h2>实时机会（最近 30 秒）</h2><table><thead><tr><th>交易对</th><th>方向</th><th>净价差</th><th>预期利润</th><th>数量</th><th>更新时间</th></tr></thead><tbody id="opps"></tbody></table>
<h2>模拟持仓</h2><table><thead><tr><th>交易对</th><th>方向</th><th>数量</th><th>入场 Z</th><th>开仓时间</th></tr></thead><tbody id="positions"></tbody></table>
<h2>最近事件</h2><table><thead><tr><th>时间</th><th>类型</th><th>交易对</th><th>原因</th><th>净利润</th></tr></thead><tbody id="events"></tbody></table>
<script>
const f=(n,d=4)=>Number(n||0).toFixed(d), t=x=>x?new Date(x).toLocaleString():'—';
async function refresh(){try{const d=await fetch('/api/status',{cache:'no-store'}).then(r=>r.json()),s=d.snapshot||{};document.querySelector('#status').textContent='服务器时间 '+t(d.server_time)+' · 页面每 2 秒刷新';document.querySelector('#cards').innerHTML=[['累计净利润',f(s.realized_pnl)+' USDT',s.realized_pnl>=0?'good':'bad'],['交易次数',s.trades||0,''],['胜率',s.trades?f(100*(s.wins||0)/s.trades,1)+'%':'—',''],['最大回撤',f(s.max_drawdown)+' USDT','bad'],['当前持仓',(s.positions||[]).length,''],['行情流',new Set((d.books||[]).map(x=>x.exchange+':'+x.symbol)).size,'']].map(x=>'<div class="card"><div class="muted">'+x[0]+'</div><div class="value '+x[2]+'">'+x[1]+'</div></div>').join('');document.querySelector('#opps').innerHTML=(d.opportunities||[]).slice(0,50).map(x=>'<tr><td>'+x.Symbol+'</td><td>'+x.BuyExchange+' → '+x.SellExchange+'</td><td class="good">'+f(x.NetEdgeBps,2)+' bp</td><td>'+f(x.ExpectedPnL)+' USDT</td><td>'+f(x.Quantity,6)+'</td><td>'+t(x.updated_at)+'</td></tr>').join('')||'<tr><td colspan="6" class="muted">暂无满足条件的机会</td></tr>';document.querySelector('#positions').innerHTML=(s.positions||[]).map(x=>'<tr><td>'+x.symbol+'</td><td>'+x.direction+'</td><td>'+f(x.quantity,6)+'</td><td>'+f(x.entry_z,2)+'</td><td>'+t(x.opened_at)+'</td></tr>').join('')||'<tr><td colspan="5" class="muted">当前无持仓</td></tr>';document.querySelector('#events').innerHTML=(d.recent_events||[]).slice().reverse().map(x=>'<tr><td>'+t(x.time)+'</td><td>'+x.type+'</td><td>'+x.position.symbol+'</td><td>'+(x.close_reason||'—')+'</td><td>'+f(x.net_pnl)+' USDT</td></tr>').join('')||'<tr><td colspan="5" class="muted">启动后暂无事件</td></tr>'}catch(e){document.querySelector('#status').textContent='连接失败：'+e.message}}
refresh();setInterval(refresh,2000);
</script></body></html>`))
