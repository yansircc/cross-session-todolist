package cst

// uiCSS is the inline stylesheet for the ui report. It mirrors the design
// tokens of llm-report-html's SCSS (light + dark via prefers-color-scheme)
// but is hand-written and embedded so cst has no runtime dependency.
const uiCSS = `
  :root{
    color-scheme: light dark;
    --md-ink:#111;--md-charcoal:#242424;
    --md-slate-800:#2b2b2b;--md-slate-700:#404040;--md-slate-600:#737373;
    --md-slate-500:#8a8a8a;--md-slate-400:#a3a3a3;--md-slate-300:#d4d4d4;
    --md-slate-200:#e5e5e5;--md-slate-100:#f1f1f1;--md-slate-50:#f9f9f9;
    --md-paper:#fff;--md-shell:#ececeb;--md-surface:#f9f9f9;--md-hover:#f1f1f1;
    --md-pond:#0055ff;--md-pond-50:#f0f6ff;
    --md-success:#008844;--md-success-50:#edf8f2;
    --md-warning:#946200;--md-warning-50:#fff7e8;
    --md-danger:#c7332f;--md-danger-50:#fff0ef;
  }
  @media (prefers-color-scheme: dark){
    :root{
      --md-ink:#f9f9f9;--md-charcoal:#e5e5e5;
      --md-slate-800:#d4d4d4;--md-slate-700:#a3a3a3;--md-slate-600:#8a8a8a;
      --md-slate-500:#737373;--md-slate-400:#404040;--md-slate-300:#2b2b2b;
      --md-slate-200:#242424;--md-slate-100:#1a1a1a;--md-slate-50:#111;
      --md-paper:#111;--md-shell:#0a0a0a;--md-surface:#1a1a1a;--md-hover:#242424;
      --md-pond-50:rgba(0,85,255,.1);
      --md-success-50:rgba(0,136,68,.1);
      --md-warning-50:rgba(148,98,0,.1);
      --md-danger-50:rgba(199,51,47,.1);
    }
  }
  *{box-sizing:border-box}
  html,body{margin:0;padding:0}
  body{
    font-family:'IBM Plex Sans',ui-sans-serif,system-ui,-apple-system,sans-serif;
    font-size:14px;line-height:1.5;-webkit-font-smoothing:antialiased;
    color:var(--md-charcoal);background:var(--md-shell);padding:32px 16px;
  }
  main{
    max-width:1000px;margin:0 auto;background:var(--md-paper);
    padding:32px 48px;border:1px solid var(--md-slate-200);border-radius:4px;
    box-shadow:0 1px 2px rgba(0,0,0,.04);
  }
  a{color:var(--md-pond);text-decoration:none}
  a:hover{text-decoration:underline}
  code{
    font-family:'JetBrains Mono',ui-monospace,Menlo,Consolas,monospace;
    font-size:12px;color:var(--md-ink);
  }
  h1{font-size:20px;font-weight:600;margin:0 0 8px;line-height:1.2;color:var(--md-ink)}
  .report-meta{
    font-family:'JetBrains Mono',ui-monospace,monospace;
    font-size:10px;color:var(--md-slate-500);
    text-transform:uppercase;letter-spacing:.05em;
    margin:0 0 12px;display:flex;gap:12px;flex-wrap:wrap;
  }
  .report-meta + .report-meta{margin-bottom:24px}
  .kbd{
    font-family:'JetBrains Mono',ui-monospace,monospace;font-size:11px;
    background:var(--md-surface);padding:1px 6px;border-radius:4px;
    border:1px solid var(--md-slate-200);color:var(--md-slate-700);
    text-transform:none;letter-spacing:0;
  }
  .scope-nav{display:flex;flex-wrap:wrap;gap:8px;margin:-8px 0 32px}
  .scope-nav a{
    font-family:'JetBrains Mono',ui-monospace,monospace;font-size:11px;
    color:var(--md-slate-600);background:var(--md-surface);
    border:1px solid var(--md-slate-200);border-radius:4px;
    padding:5px 10px;display:inline-flex;align-items:center;gap:8px;
    text-decoration:none;
  }
  .scope-nav a:hover{background:var(--md-hover);color:var(--md-ink);text-decoration:none}
  .scope-nav .nv-bar{display:inline-block;width:30px;height:3px;
    background:var(--md-slate-200);border-radius:2px;overflow:hidden}
  .scope-nav .nv-bar > i{display:block;height:100%;background:var(--md-success)}
  .scope-nav .nv-frac{color:var(--md-ink);font-weight:500}
  .kpi-grid{display:grid;grid-template-columns:repeat(4,1fr);gap:12px;margin:16px 0 32px}
  .stat-card{
    background:var(--md-surface);border-radius:4px;padding:10px 14px;
    display:flex;flex-direction:column;gap:4px;
  }
  .stat-label{
    font-family:'JetBrains Mono',ui-monospace,monospace;
    color:var(--md-slate-500);font-size:10px;
    text-transform:uppercase;letter-spacing:.06em;
  }
  .stat-value{
    font-size:15px;font-weight:600;color:var(--md-ink);line-height:1.15;
    font-variant-numeric:tabular-nums;
  }
  .run-diagnostics{
    display:grid;grid-template-columns:1fr;gap:10px;margin:-12px 0 28px;
  }
  .run-list{
    background:var(--md-surface);border:1px solid var(--md-slate-200);
    border-radius:4px;padding:10px 12px;
  }
  .run-list-title{
    font-family:'JetBrains Mono',ui-monospace,monospace;font-size:10px;
    color:var(--md-slate-500);text-transform:uppercase;letter-spacing:.06em;
    margin-bottom:6px;font-weight:600;
  }
  .run-line{
    display:flex;align-items:baseline;gap:6px;flex-wrap:wrap;
    font-family:'JetBrains Mono',ui-monospace,monospace;font-size:11px;
    color:var(--md-slate-600);line-height:1.7;
  }
  .run-node,.run-at{
    color:var(--md-slate-500);font-size:10px;text-transform:uppercase;
    letter-spacing:.04em;
  }
  .run-line code{
    background:var(--md-paper);border:1px solid var(--md-slate-200);
    padding:1px 5px;border-radius:3px;color:var(--md-ink);font-size:11px;
    word-break:break-all;
  }
  .run-line .cmd,.verify .cmd{
    color:var(--md-pond);background:var(--md-pond-50);
  }
  .run-line .ok{color:var(--md-success);font-weight:600}
  .run-line .fail{color:var(--md-danger);font-weight:600}
  .run-line .trig{background:var(--md-slate-100);color:var(--md-slate-700);padding:1px 5px;border-radius:3px}
  section.scope{margin:24px 0;scroll-margin-top:24px}
  .callout{
    border:1px solid var(--md-slate-200);border-radius:4px;
    background:var(--md-paper);overflow:hidden;margin-bottom:16px;
    box-shadow:0 1px 2px rgba(0,0,0,.02);
  }
  .callout-title{
    display:flex;align-items:center;gap:10px;padding:10px 14px;
    font-size:13px;font-weight:600;color:var(--md-ink);
    border-bottom:1px solid var(--md-slate-200);background:var(--md-surface);
  }
  .callout-title::before{
    content:"";width:8px;height:8px;border-radius:50%;
    background:var(--md-slate-400);flex-shrink:0;
  }
  .callout.info .callout-title{background:var(--md-pond-50);border-bottom-color:rgba(0,85,255,.1)}
  .callout.info .callout-title::before{background:var(--md-pond)}
  .callout-body{padding:14px 18px;font-size:13px}
  .callout-body > :first-child{margin-top:0}
  .callout-body > :last-child{margin-bottom:0}
  .scope-title{flex:1;min-width:0}
  .scope-title .nid{
    font-family:'JetBrains Mono',ui-monospace,monospace;
    font-size:11px;font-weight:400;color:var(--md-slate-500);margin-right:8px;
  }
  .scope-meta{
    margin-left:auto;font-family:'JetBrains Mono',ui-monospace,monospace;
    font-size:10px;color:var(--md-slate-500);font-weight:400;
    text-transform:uppercase;letter-spacing:.05em;
  }
  .crumb{
    font-family:'JetBrains Mono',ui-monospace,monospace;font-size:10px;
    color:var(--md-slate-500);text-transform:uppercase;letter-spacing:.05em;
    margin-bottom:12px;
  }
  .crumb a{color:var(--md-slate-500)}
  .progress{margin:0 0 16px}
  .bar{height:4px;background:var(--md-slate-100);border-radius:2px;overflow:hidden}
  .bar > div{height:100%;background:var(--md-success);border-radius:2px}
  .bar-line{
    display:flex;justify-content:space-between;align-items:baseline;
    font-family:'JetBrains Mono',ui-monospace,monospace;font-size:10px;
    color:var(--md-slate-500);text-transform:uppercase;letter-spacing:.05em;
    margin-top:8px;flex-wrap:wrap;gap:8px;
  }
  .bar-line b{color:var(--md-ink);font-weight:600;font-variant-numeric:tabular-nums}
  .counts span{margin-left:12px}
  .rules-block{
    background:var(--md-surface);border:1px solid var(--md-slate-200);
    border-radius:4px;padding:10px 14px;margin:0 0 16px;
  }
  .rules-label{
    font-family:'JetBrains Mono',ui-monospace,monospace;font-size:10px;
    color:var(--md-slate-500);text-transform:uppercase;letter-spacing:.06em;
    margin-bottom:6px;
  }
  .rules-block ul{margin:0;padding-left:20px;font-size:12px;color:var(--md-slate-700)}
  .rules-block li{margin:4px 0}
  .rule-id{
    font-family:'JetBrains Mono',ui-monospace,monospace;font-size:10px;
    color:var(--md-slate-500);margin-right:4px;
  }
  .sub-scopes{
    font-family:'JetBrains Mono',ui-monospace,monospace;font-size:10px;
    color:var(--md-slate-500);text-transform:uppercase;letter-spacing:.05em;
    margin:0 0 16px;
  }
  .sub-scopes a{color:var(--md-pond)}
  .sub-scopes .done-ref{color:var(--md-slate-400);text-decoration:line-through}
  .group-label{
    font-family:'JetBrains Mono',ui-monospace,monospace;font-size:10px;
    color:var(--md-slate-500);text-transform:uppercase;letter-spacing:.06em;
    font-weight:500;margin:20px 0 8px;padding-top:8px;
    border-top:1px solid var(--md-slate-100);
  }
  .group-label:first-of-type{border-top:none;padding-top:0;margin-top:0}
  .task{
    border:1px solid var(--md-slate-200);border-radius:4px;
    background:var(--md-paper);margin:8px 0;overflow:hidden;
  }
  .task-head{
    display:flex;align-items:center;gap:10px;padding:8px 12px;
    border-bottom:1px solid var(--md-slate-200);background:var(--md-surface);
    flex-wrap:wrap;
  }
  .task-head::before{
    content:"";width:6px;height:6px;border-radius:50%;
    background:var(--md-slate-400);flex-shrink:0;
  }
  .task.done .task-head{background:var(--md-success-50);border-bottom-color:rgba(0,136,68,.1)}
  .task.done .task-head::before{background:var(--md-success)}
  .task.claimed .task-head{background:var(--md-pond-50);border-bottom-color:rgba(0,85,255,.1)}
  .task.claimed .task-head::before{background:var(--md-pond)}
  .task.held .task-head{background:var(--md-warning-50);border-bottom-color:rgba(148,98,0,.1)}
  .task.held .task-head::before{background:var(--md-warning)}
  .task.abandoned .task-head{background:var(--md-danger-50);border-bottom-color:rgba(199,51,47,.1)}
  .task.abandoned .task-head::before{background:var(--md-danger)}
  .task-status{
    font-family:'JetBrains Mono',ui-monospace,monospace;font-size:10px;
    text-transform:uppercase;letter-spacing:.06em;font-weight:600;
  }
  .task.done .task-status{color:var(--md-success)}
  .task.claimed .task-status{color:var(--md-pond)}
  .task.held .task-status{color:var(--md-warning)}
  .task.abandoned .task-status{color:var(--md-danger)}
  .task.ready .task-status{color:var(--md-slate-600)}
  .task-nid{
    font-family:'JetBrains Mono',ui-monospace,monospace;font-size:10px;
    color:var(--md-slate-500);
  }
  .task-title{font-size:13px;color:var(--md-ink);font-weight:500;flex:1;min-width:240px}
  .task-when{
    font-family:'JetBrains Mono',ui-monospace,monospace;font-size:10px;
    color:var(--md-slate-500);text-transform:uppercase;letter-spacing:.05em;
    margin-left:auto;
  }
  .task-body{padding:12px 14px;font-size:13px}
  .task-body > :first-child{margin-top:0}
  .task-body > :last-child{margin-bottom:0}
  .task-body > * + *{margin-top:10px}
  .commands{
    display:flex;align-items:center;gap:6px;flex-wrap:wrap;
    font-family:'JetBrains Mono',ui-monospace,monospace;font-size:11px;
    color:var(--md-slate-600);
  }
  .commands .field-label{width:100%;margin-bottom:0}
  .commands code{
    background:var(--md-surface);border:1px solid var(--md-slate-200);
    padding:2px 6px;border-radius:3px;color:var(--md-pond);font-size:11px;
    word-break:break-all;
  }
  .note{
    background:var(--md-surface);border:1px solid var(--md-slate-200);
    border-radius:4px;padding:10px 12px;font-size:12px;
    line-height:1.55;color:var(--md-slate-700);
    white-space:pre-wrap;word-break:break-word;
  }
  .field-label{
    display:block;
    font-family:'JetBrains Mono',ui-monospace,monospace;font-size:10px;
    color:var(--md-slate-500);text-transform:uppercase;letter-spacing:.06em;
    margin-bottom:4px;font-weight:500;
  }
  blockquote.reason{
    margin:0;padding:4px 16px;font-size:12px;line-height:1.6;
    border-left:3px solid var(--md-warning);color:var(--md-slate-700);
  }
  blockquote.reason.aban{border-left-color:var(--md-danger)}
  blockquote.reason .field-label{margin-bottom:2px}
  .verify{
    font-family:'JetBrains Mono',ui-monospace,monospace;font-size:11px;
    color:var(--md-slate-600);line-height:1.7;
  }
  .verify code{background:var(--md-surface);border:1px solid var(--md-slate-200);
    padding:1px 5px;border-radius:3px;color:var(--md-ink);font-size:11px;word-break:break-all}
  .verify .ok{color:var(--md-success);font-weight:600}
  .verify .fail{color:var(--md-danger);font-weight:600}
  .verify .trig{background:var(--md-slate-100);color:var(--md-slate-700);padding:1px 5px;border-radius:3px}
  .verify .truncated{color:var(--md-warning);margin-left:6px}
  .run-detail{padding:8px 12px;background:var(--md-paper)}
  .run-more{
    font-family:'JetBrains Mono',ui-monospace,monospace;font-size:10px;
    color:var(--md-slate-500);text-transform:uppercase;letter-spacing:.05em;
    margin-bottom:4px;
  }
  .acceptance{
    font-family:'JetBrains Mono',ui-monospace,monospace;font-size:11px;
    color:var(--md-slate-600);
  }
  .acceptance code{background:var(--md-surface);border:1px solid var(--md-slate-200);
    padding:1px 5px;border-radius:3px;color:var(--md-ink);font-size:11px;word-break:break-all}
  details{
    background:var(--md-surface);border:1px solid var(--md-slate-200);
    border-radius:4px;margin:0;overflow:hidden;
  }
  details[open]{background:var(--md-paper)}
  details[open] summary{border-bottom:1px solid var(--md-slate-200)}
  details[open] summary::after{transform:rotate(135deg)}
  summary{
    font-family:'JetBrains Mono',ui-monospace,monospace;font-size:11px;
    font-weight:500;cursor:pointer;padding:8px 12px;list-style:none;
    display:flex;align-items:center;justify-content:space-between;gap:12px;
    color:var(--md-slate-700);
  }
  summary:hover{background:var(--md-hover)}
  summary::-webkit-details-marker{display:none}
  summary::after{
    content:"";width:5px;height:5px;
    border-top:1px solid currentColor;border-right:1px solid currentColor;
    transform:rotate(45deg);transition:transform .2s;opacity:.5;flex-shrink:0;
  }
  details pre{
    margin:0;padding:12px;background:var(--md-paper);color:var(--md-ink);
    border:none;border-radius:0;font-family:'JetBrains Mono',ui-monospace,monospace;
    font-size:11px;line-height:1.55;overflow:auto;max-height:300px;
    white-space:pre-wrap;word-break:break-all;
  }
  details dl.ev-data{
    margin:0;padding:10px 12px;background:var(--md-paper);font-size:11px;
    color:var(--md-slate-700);
  }
  details dl.ev-data dt{
    font-family:'JetBrains Mono',ui-monospace,monospace;font-weight:600;
    color:var(--md-slate-500);font-size:10px;
    text-transform:uppercase;letter-spacing:.05em;margin-top:8px;
  }
  details dl.ev-data dt:first-child{margin-top:0}
  details dl.ev-data dd{
    margin:2px 0 0;font-family:'JetBrains Mono',ui-monospace,monospace;
    font-size:11px;word-break:break-word;color:var(--md-ink);
  }
  .empty{
    color:var(--md-slate-500);font-style:italic;padding:24px 0;font-size:13px;
    text-align:center;
  }
  footer{
    margin-top:48px;padding-top:16px;border-top:1px solid var(--md-slate-100);
    font-family:'JetBrains Mono',ui-monospace,monospace;font-size:10px;
    color:var(--md-slate-500);text-transform:uppercase;letter-spacing:.05em;
  }
  @media (max-width:640px){
    body{padding:16px 8px}
    main{padding:20px 18px}
    .kpi-grid{grid-template-columns:repeat(2,1fr)}
  }
`
