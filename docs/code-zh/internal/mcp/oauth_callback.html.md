```html
<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <meta name="color-scheme" content="light dark">
  <title>{{.标题}} · Callum</title>
  <script nonce="{{.随机数}}">
    const 干净地址 = new URL(位置.完整地址);
    干净地址.查询字符串 = '';
    干净地址.片段 = '';
    历史记录.替换状态(null, '', 干净地址.完整地址);
  </script>
  <style>
    :root {
      color-scheme: light dark;
      --背景: #f7f7fb;
      --光晕: #ece6ff;
      --表面: #fff;
      --文字: #211d30;
      --弱化文字: #626071;
      --边线: #e7e4f0;
      --品牌: #6651cf;
      --品牌-浅色: #f0ebff;
      --阴影: 0 24px 72px rgba(45, 35, 80, .09), 0 3px 12px rgba(45, 35, 80, .04);
      font: 16px/1.65 system-ui, -apple-system, BlinkMacSystemFont, "PingFang SC", "Microsoft YaHei", sans-serif;
    }

    * { box-sizing: border-box; }

    body {
      min-height: 100vh;
      min-height: 100svh;
      display: grid;
      place-items: center;
      margin: 0;
      padding: 28px 20px;
      color: var(--文字);
      background: radial-gradient(ellipse at 50% 14%, var(--光晕), transparent 60%), var(--背景);
    }

    main {
      width: min(100%, 460px);
      padding: clamp(22px, 7vw, 40px);
      border: 1px solid var(--边线);
      border-radius: 26px;
      background: var(--表面);
      box-shadow: var(--阴影);
    }

    .brand {
      display: inline-flex;
      align-items: center;
      gap: 9px;
      font-size: 15px;
      font-weight: 650;
      letter-spacing: -.02em;
    }

    .brand-mark {
      display: grid;
      place-items: center;
      width: 26px;
      height: 26px;
      border-radius: 8px;
      color: #fff;
      background: #6651cf;
      font-size: 15px;
      font-weight: 720;
    }

    .message { margin: 34px 0 30px; }

    .icon {
      --图标: var(--品牌);
      --图标-浅色: var(--品牌-浅色);
      display: grid;
      place-items: center;
      width: 56px;
      height: 56px;
      color: var(--图标);
      background: var(--图标-浅色);
      border-radius: 18px;
      margin-bottom: 16px;
    }

    .icon::before {
      content: "";
      width: 23px;
      height: 12px;
      transform: translateY(-2px) rotate(-45deg);
      border: solid currentColor;
      border-width: 0 0 3px 3px;
    }

    [data-status="cancelled"] .icon {
      --图标: var(--弱化文字);
      --图标-浅色: var(--背景);
    }

    [data-status="cancelled"] .icon::before {
      width: 20px;
      height: 0;
      transform: none;
      border-width: 0 0 3px;
      border-radius: 8px;
    }

    [data-status="failed"] .icon {
      --图标: #a72647;
      --图标-浅色: #ffeaf0;
    }

    [data-status="failed"] .icon::before {
      content: "!";
      width: auto;
      height: auto;
      border: 0;
      transform: none;
      font-size: 27px;
      line-height: 1;
      font-weight: 740;
    }

    h1 {
      font-size: clamp(24px, 7vw, 32px);
      line-height: 1.3;
      font-weight: 680;
      letter-spacing: -.035em;
      margin: 0 0 10px;
    }

    p { margin: 0; color: var(--弱化文字); overflow-wrap: anywhere; }
    strong { font-weight: 620; color: var(--文字); }

    footer {
      padding-top: 18px;
      border-top: 1px solid var(--边线);
      font-size: 14px;
    }

    @media (prefers-color-scheme: dark) {
      :root {
        --背景: #16141d;
        --光晕: #28213c;
        --表面: #211e2b;
        --文字: #f3eefb;
        --弱化文字: #b9b1c8;
        --边线: #393245;
        --品牌: #c4b5ff;
        --品牌-浅色: #312748;
        --阴影: 0 22px 70px rgba(7, 5, 13, .3);
      }
      [data-status="failed"] .icon {
        --图标: #ffa6bc;
        --图标-浅色: #442834;
      }
    }

    @media (max-width: 400px) {
      .message { margin-block: 27px 24px; }
    }
  </style>
</head>
<body>
  <main data-status="{{.状态}}" aria-labelledby="result-title">
    <header class="brand"><span class="brand-mark" aria-hidden="true">C</span>Callum</header>
    <section class="message" aria-labelledby="result-title">
      <span class="icon" aria-hidden="true"></span>
      <h1 id="result-title">{{.标题}}</h1>
      {{if eq .状态 "authorized"}}
      <p>已为 <strong>{{.服务}}</strong> 保存访问凭据。</p>
      {{else}}
      <p>{{.消息}}</p>
      {{end}}
    </section>
    <footer><p>{{.提示}}</p></footer>
  </main>
</body>
</html>
```
