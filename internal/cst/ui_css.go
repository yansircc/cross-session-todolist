package cst

// uiCSS is the inline stylesheet for the ui report. It intentionally keeps the
// surface narrow: progress index, one selected task detail, and folded context.
const uiCSS = `
  :root{
    color-scheme:light dark;
    --bg:#ffffff;
    --ink:#111111;
    --muted:#6b7280;
    --line:#f0f0f0;
    --soft:#fafafa;
    --accent:#2563eb;
    --accent-soft:#eff6ff;
    --success:#10b981;
    --success-soft:#ecfdf5;
    --warning:#b45309;
    --mono:ui-monospace,SFMono-Regular,Menlo,Monaco,Consolas,"Liberation Mono",monospace;
    --sans:Inter,ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;
  }
  @media (prefers-color-scheme:dark){
    :root{
      --bg:#0b0f14;
      --ink:#f3f4f6;
      --muted:#9ca3af;
      --line:#1f2937;
      --soft:#111827;
      --accent:#60a5fa;
      --accent-soft:#172554;
      --success:#34d399;
      --success-soft:#052e2b;
      --warning:#fbbf24;
    }
  }
  *{box-sizing:border-box}
  html,body{margin:0;min-height:100%}
  body{
    background:var(--bg);
    color:var(--ink);
    font-family:var(--sans);
    font-size:14px;
    line-height:1.5;
    letter-spacing:0;
    -webkit-font-smoothing:antialiased;
  }
  code,.mono{font-family:var(--mono);font-size:13px;letter-spacing:0}
  .task-radio{
    position:absolute;
    width:1px;
    height:1px;
    overflow:hidden;
    clip:rect(0 0 0 0);
    white-space:nowrap;
  }
  .page{
    max-width:880px;
    margin:0 auto;
    padding:48px 24px;
  }
  .top{
    display:flex;
    justify-content:space-between;
    align-items:flex-start;
    gap:24px;
    padding-bottom:32px;
    border-bottom:1px solid var(--line);
    margin-bottom:32px;
  }
  h1{
    margin:0 0 12px;
    font-size:20px;
    line-height:1.2;
    font-weight:600;
    letter-spacing:0;
  }
  .meta{
    display:flex;
    gap:8px;
    align-items:center;
    flex-wrap:wrap;
  }
  .tag{
    font-size:12px;
    color:var(--muted);
    font-weight:400;
  }
  .tag.active{color:var(--accent)}
  .progress-total{
    font-size:32px;
    line-height:1;
    font-weight:300;
    letter-spacing:0;
    white-space:nowrap;
  }
  main{display:grid;gap:36px}
  .phase{
    position:relative;
    display:block;
    padding-bottom:28px;
    border-bottom:1px solid var(--line);
  }
  .phase:last-child{border-bottom:0}
  .phase-head{
    display:flex;
    justify-content:space-between;
    align-items:center;
    gap:20px;
    margin-bottom:24px;
  }
  .phase h2{
    margin:0;
    font-size:16px;
    line-height:1.3;
    font-weight:500;
    letter-spacing:0;
  }
  .progress{
    display:flex;
    flex-direction:column;
    align-items:flex-end;
    gap:8px;
  }
  .progress b{
    font-size:14px;
    font-weight:400;
    color:var(--muted);
  }
  .steps{
    display:flex;
    flex-wrap:wrap;
    justify-content:flex-end;
    gap:6px;
  }
  .step{
    width:9px;
    height:9px;
    border-radius:2px;
    background:#e5e7eb;
    cursor:pointer;
    transition:background 150ms ease,transform 150ms ease;
  }
  .step.done{background:var(--success)}
  .step.now{background:var(--accent)}
  .step:hover{background:var(--ink)}
  .detail{padding-top:2px}
  .panel{display:none}
  .panel h3{
    margin:0 0 8px;
    font-size:22px;
    line-height:1.25;
    font-weight:600;
    letter-spacing:0;
  }
  .state-line{
    margin-bottom:14px;
    color:var(--muted);
    font-size:13px;
  }
  .state-line.ready,
  .state-line.review,
  .state-line.claimed{color:var(--accent)}
  .state-line.done{color:var(--success)}
  .state-line.held,
  .state-line.failed{color:var(--warning)}
  .brief{
    max-width:70ch;
    margin-bottom:28px;
    color:var(--ink);
    font-size:16px;
    line-height:1.6;
  }
  .grid{display:grid;grid-template-columns:1fr;gap:0}
  details{
    border-top:1px solid var(--line);
    padding:16px 0;
  }
  details:last-child{border-bottom:1px solid var(--line)}
  summary{
    list-style:none;
    cursor:pointer;
    display:flex;
    justify-content:space-between;
    align-items:center;
    gap:16px;
    color:var(--ink);
    font-size:14px;
    font-weight:500;
    user-select:none;
  }
  summary::-webkit-details-marker{display:none}
  .summary-left{
    display:flex;
    align-items:center;
    gap:12px;
  }
  .count{
    color:var(--muted);
    font-size:13px;
    font-weight:400;
  }
  summary::after{
    content:"+";
    color:var(--muted);
    font-size:18px;
    font-weight:300;
    line-height:1;
  }
  details[open] summary::after{content:"-"}
  .detail-body{
    display:grid;
    gap:12px;
    padding-top:16px;
    padding-left:24px;
    color:var(--muted);
    font-size:14px;
  }
  .row{
    display:grid;
    grid-template-columns:70px minmax(0,1fr);
    gap:16px;
    align-items:start;
  }
  .row span:first-child{
    color:var(--ink);
    font-size:12px;
    font-weight:500;
  }
  .muted,.more{color:var(--muted)}
  .checks{
    display:flex;
    flex-wrap:wrap;
    gap:6px;
  }
  .check{
    display:inline-flex;
    align-items:center;
    min-height:22px;
    border-radius:4px;
    background:var(--soft);
    color:var(--muted);
    padding:2px 8px;
    font-size:12px;
  }
  .check.pass{
    background:var(--success-soft);
    color:var(--success);
  }
  .check.next{
    background:var(--accent-soft);
    color:var(--accent);
  }
  .empty{
    padding:24px 0;
    color:var(--muted);
    font-style:italic;
    text-align:center;
  }
  @media(max-width:760px){
    .page{padding:32px 16px}
    .top,.phase-head{
      flex-direction:column;
      align-items:flex-start;
    }
    .progress{align-items:flex-start}
    .steps{justify-content:flex-start}
    .detail-body{padding-left:0}
    .brief{font-size:15px}
  }
`
