package cst

// uiCSS is the inline stylesheet for the ui report. It follows the
// zeroY-design "Utility-First Colorful Docs + Functional Brutalism" direction:
// dense borders, docs-like rails, semantic color lanes, and no UI-owned state.
const uiCSS = `
  :root{
    color-scheme:light dark;
    --bg:#f8fafc;--paper:#fff;--paper-2:#f1f5f9;
    --ink:#0f172a;--muted:#64748b;--quiet:#94a3b8;
    --line:#cbd5e1;--line-soft:#e2e8f0;
    --sky:#0284c7;--sky-soft:#e0f2fe;
    --teal:#0f766e;--teal-soft:#ccfbf1;
    --amber:#b45309;--amber-soft:#fef3c7;
    --rose:#be123c;--rose-soft:#ffe4e6;
    --violet:#6d28d9;--violet-soft:#ede9fe;
    --mono:ui-monospace,SFMono-Regular,Menlo,Monaco,Consolas,"Liberation Mono",monospace;
    --sans:Inter,ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;
  }
  @media (prefers-color-scheme:dark){
    :root{
      --bg:#0b0f14;--paper:#111827;--paper-2:#0f172a;
      --ink:#f8fafc;--muted:#cbd5e1;--quiet:#94a3b8;
      --line:#334155;--line-soft:#1e293b;
      --sky-soft:rgba(2,132,199,.18);
      --teal-soft:rgba(15,118,110,.2);
      --amber-soft:rgba(180,83,9,.18);
      --rose-soft:rgba(190,18,60,.18);
      --violet-soft:rgba(109,40,217,.2);
    }
  }
  *{box-sizing:border-box}
  html,body{margin:0;min-height:100%}
  body{
    background:var(--bg);color:var(--ink);font-family:var(--sans);
    font-size:14px;line-height:1.45;letter-spacing:0;
  }
  code,.mono{font-family:var(--mono);font-size:12px;letter-spacing:0}
  a{color:var(--sky);text-decoration:none}
  a:hover{text-decoration:underline}
  .layout{display:grid;grid-template-columns:260px minmax(0,1fr) 320px;min-height:100vh}
  .rail{
    border-right:1px solid var(--line);background:var(--paper);
    padding:18px 16px;position:sticky;top:0;height:100vh;overflow:auto;
  }
  .brand{display:grid;gap:8px;margin-bottom:22px}
  .brand h1{margin:0;font-size:17px;line-height:1.18;font-weight:760}
  .brand .root{color:var(--muted);overflow-wrap:anywhere}
  .nav-list{display:grid;gap:6px}
  .nav-item{
    display:grid;grid-template-columns:42px minmax(0,1fr) auto;gap:8px;align-items:center;
    border:1px solid var(--line-soft);background:var(--paper);border-radius:4px;
    padding:8px;color:var(--ink);
  }
  .nav-item.active{border-color:var(--sky);background:var(--sky-soft)}
  .nav-item .name{overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-weight:650}
  .dot{width:8px;height:8px;border-radius:99px;background:var(--quiet);color:transparent;font-size:0;line-height:0}
  .dot.claimed{background:var(--sky)}
  .dot.ready{background:var(--teal)}
  .dot.review{background:var(--violet)}
  .dot.waiting,.dot.held{background:var(--amber)}
  .dot.failed{background:var(--rose)}
  .dot.done{background:var(--teal)}
  .empty-rail,.rail-footer{margin-top:22px;border-top:1px solid var(--line-soft);padding-top:12px;color:var(--muted)}
  .content{min-width:0;padding:24px}
  .top{display:grid;grid-template-columns:minmax(0,1fr) auto;gap:16px;align-items:end;margin-bottom:18px}
  .eyebrow{color:var(--sky);font-weight:760;text-transform:uppercase;font-size:11px;margin-bottom:6px}
  .title{
    margin:0;font-size:clamp(28px,4vw,56px);line-height:.95;font-weight:820;
    text-wrap:pretty;
  }
  .meta{display:flex;flex-wrap:wrap;gap:8px;margin-top:12px;color:var(--muted)}
  .chip{
    display:inline-flex;align-items:center;min-height:24px;border:1px solid var(--line);
    background:var(--paper);border-radius:4px;padding:2px 7px;white-space:nowrap;
  }
  .refresh{display:flex;gap:8px;flex-wrap:wrap;justify-content:flex-end}
  .cmd-pill{
    border:1px solid var(--line);background:var(--paper);color:var(--ink);
    border-radius:4px;padding:7px 10px;font-weight:720;min-height:34px;
    display:inline-flex;align-items:center;
  }
  .cmd-pill.primary{background:var(--ink);color:var(--paper);border-color:var(--ink)}
  .progress-strip{
    display:grid;grid-template-columns:1.5fr repeat(5,minmax(0,1fr));
    border:1px solid var(--line);background:var(--paper);border-radius:4px;
    overflow:hidden;margin-bottom:18px;
  }
  .metric{padding:12px;border-right:1px solid var(--line-soft);min-height:92px}
  .metric:last-child{border-right:0}
  .metric .label{
    color:var(--muted);font-size:11px;text-transform:uppercase;font-weight:760;margin-bottom:8px;
  }
  .metric .value{display:flex;align-items:baseline;gap:8px;font-size:28px;font-weight:820;line-height:1}
  .metric .value span{color:var(--muted);font-size:12px;font-weight:650}
  .metric .note{color:var(--muted);margin-top:8px;font-size:12px}
  .metric.primary{background:var(--sky-soft)}
  .phase-section{border:1px solid var(--line);border-radius:4px;background:var(--paper);overflow:hidden}
  .section-head{
    display:grid;grid-template-columns:minmax(0,1fr) auto;gap:12px;
    padding:12px 14px;border-bottom:1px solid var(--line);background:var(--paper-2);
  }
  .section-head h2{margin:0;font-size:15px}
  .section-head .sub{color:var(--muted);margin-top:3px;font-size:12px}
  .phase{border-bottom:1px solid var(--line)}
  .phase:last-child{border-bottom:0}
  .phase>summary{list-style:none;cursor:pointer}
  .phase>summary::-webkit-details-marker{display:none}
  .phase-row{
    display:grid;grid-template-columns:76px minmax(240px,1fr) 160px 280px;
    gap:14px;align-items:center;padding:14px;
  }
  .phase-row:hover{background:var(--paper-2)}
  .pid{display:grid;gap:6px;color:var(--muted)}
  .phase[open] .pid::after{content:"open";color:var(--sky);font-family:var(--mono);font-size:11px}
  .phase:not([open]) .pid::after{content:"closed";color:var(--quiet);font-family:var(--mono);font-size:11px}
  .phase-title{font-weight:760;overflow-wrap:anywhere;margin-bottom:7px}
  .facts{display:flex;flex-wrap:wrap;gap:6px}
  .badge{
    display:inline-flex;align-items:center;min-height:23px;border-radius:4px;
    padding:2px 7px;font-weight:720;font-size:12px;white-space:nowrap;
  }
  .badge.claimed{background:var(--sky-soft);color:var(--sky)}
  .badge.ready{background:var(--teal-soft);color:var(--teal)}
  .badge.waiting,.badge.held{background:var(--amber-soft);color:var(--amber)}
  .badge.review{background:var(--violet-soft);color:var(--violet)}
  .badge.failed{background:var(--rose-soft);color:var(--rose)}
  .badge.done{background:var(--teal-soft);color:var(--teal)}
  .ratio{display:grid;gap:8px}
  .ratio-line{display:flex;justify-content:space-between;gap:8px;color:var(--muted);font-weight:680}
  .ratio-line strong{color:var(--ink);font-size:22px;line-height:1}
  .bar{height:8px;background:linear-gradient(90deg,var(--teal) var(--pct),var(--line-soft) 0);border-radius:99px;overflow:hidden}
  .blocker{border-left:3px solid var(--amber);padding-left:10px;color:var(--muted);font-size:12px}
  .blocker strong{display:block;color:var(--ink);margin-bottom:4px}
  .phase-detail{border-top:1px solid var(--line-soft);background:var(--paper-2);padding:14px}
  .detail-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:12px;margin-bottom:14px}
  .detail{background:var(--paper);border:1px solid var(--line-soft);border-radius:4px;padding:10px}
  .detail h3{margin:0 0 8px;font-size:13px}
  .detail ul{margin:0;padding-left:18px;color:var(--muted);font-size:12px}
  .detail li+li{margin-top:4px}
  .cmd{
    display:block;width:100%;border:1px solid var(--line);background:var(--paper-2);
    color:var(--sky);border-radius:4px;padding:5px 7px;overflow-wrap:anywhere;margin-top:6px;
  }
  table{width:100%;border-collapse:collapse;background:var(--paper);border:1px solid var(--line);font-size:12px}
  th,td{border-bottom:1px solid var(--line-soft);padding:8px;text-align:left;vertical-align:top}
  th{color:var(--muted);background:var(--paper-2);font-weight:760;text-transform:uppercase;font-size:11px}
  tr:last-child td{border-bottom:0}
  td.task-name{color:var(--ink);font-weight:650;width:34%}
  .state-detail{color:var(--muted);margin-top:4px;font-size:11px}
  .table-more{color:var(--muted);font-family:var(--mono)}
  .task-detail{margin-top:6px}
  .task-detail summary{cursor:pointer;color:var(--sky);font-weight:720;list-style:none;width:fit-content}
  .task-detail summary::-webkit-details-marker{display:none}
  .task-detail p{color:var(--muted);margin:6px 0 0}
  .side{
    border-left:1px solid var(--line);background:var(--paper);padding:18px 16px;
    position:sticky;top:0;height:100vh;overflow:auto;
  }
  .side-block{border-bottom:1px solid var(--line-soft);padding-bottom:16px;margin-bottom:16px}
  .side-block:last-child{border-bottom:0;margin-bottom:0;padding-bottom:0}
  .side-block h2{margin:0 0 10px;font-size:14px}
  .side-muted{color:var(--muted)}
  .kv{display:grid;grid-template-columns:92px minmax(0,1fr);gap:7px 10px;color:var(--muted);font-size:12px}
  .kv .v{color:var(--ink);overflow-wrap:anywhere}
  .event-list{margin:0;padding:0;list-style:none;display:grid;gap:10px}
  .event-list li{color:var(--muted)}
  .event-list strong{color:var(--ink);display:block;margin-bottom:2px}
  .empty{color:var(--muted);font-style:italic;padding:24px;text-align:center}
  @media (max-width:1220px){
    .layout{grid-template-columns:220px minmax(0,1fr)}
    .side{position:static;height:auto;grid-column:1/-1;border-left:0;border-top:1px solid var(--line)}
    .progress-strip{grid-template-columns:repeat(3,minmax(0,1fr))}
    .metric:nth-child(3n){border-right:0}
    .phase-row{grid-template-columns:76px minmax(220px,1fr) 150px}
    .blocker{grid-column:2/-1}
    .detail-grid{grid-template-columns:1fr}
  }
  @media (max-width:820px){
    .layout{display:block}
    .rail,.side{position:static;height:auto;border:0;border-bottom:1px solid var(--line)}
    .content{padding:16px}
    .top{grid-template-columns:1fr}
    .refresh{justify-content:flex-start}
    .progress-strip{grid-template-columns:repeat(2,minmax(0,1fr))}
    .metric{border-right:1px solid var(--line-soft)}
    .phase-row{grid-template-columns:1fr}
    .blocker{grid-column:auto}
    table{display:block;overflow-x:auto}
  }
`
