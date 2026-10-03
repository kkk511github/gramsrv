const assert = require('node:assert/strict');
const { chromium } = require('playwright');

(async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    const page = await browser.newPage({ viewport: { width: 1365, height: 1000 } });
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    let lastAction;
    await page.route('**/api/**', async route => {
      const url = new URL(route.request().url());
      let body;
      if (url.pathname === '/api/session') body = { actor: 'UI test', permissions: ['*'] };
      else if (url.pathname === '/api/server/identity') body = { name: 'SafeLink', description: '', default_welcome_message_phone_template: '', default_welcome_message_email_template: '', default_login_code_message_template: '{{code}}' };
      else if (url.pathname === '/api/server/env') body = [];
      else if (url.pathname === '/api/server/login-policy') body = { future_auth_enabled: true, future_auth_days: 30, registration_password_required: false };
      else if (url.pathname === '/api/actions/login-policy') {
        lastAction = route.request().postDataJSON();
        body = { status: 'completed', dry_run: !lastAction.confirm, command_id: 'ui-test-policy', message: '登录设置测试成功', details: {} };
      }
      else if (url.pathname === '/api/server/domains') body = { public_url: 'https://safelink.chat', web_url: 'https://web.safelink.chat', editable: true };
      else if (url.pathname === '/api/server/registration-invites') body = { enabled: false, items: [{ id: 1, prefix: '012', code: '01234', used_count: 12, max_uses: 100, expires_at: '2027-01-01T00:00:00Z', disabled: false }, {id: 2, prefix: 'SAF', used_count: 0, max_uses: 1, expires_at: '2027-01-01T00:00:00Z', disabled: false}] };
      else if (url.pathname === '/api/actions/registration-invites') {
        lastAction = route.request().postDataJSON();
        body = { status: 'completed', dry_run: !lastAction.confirm, command_id: 'ui-test', message: '测试成功', details: lastAction.confirm ? { codes: '01234' } : {} };
      } else { await route.fulfill({ status: 503, json: { error: 'Unavailable in isolated UI test' } }); return; }
      await route.fulfill({ json: body });
    });
    await page.goto(process.env.ADMIN_UI_URL || 'http://127.0.0.1:2419/server-settings?focus=invites');
    const section = page.locator('#serversettings-invites');
    await section.getByRole('button', { name: '生成邀请码', exact: true }).waitFor();
    await section.scrollIntoViewIfNeeded();
    await section.getByLabel('固定邀请码', { exact: true }).fill('12A45');
    assert.equal(await section.getByRole('button', { name: '生成邀请码', exact: true }).isDisabled(), true);
    await section.getByLabel('固定邀请码', { exact: true }).fill('01234');
    assert.equal(await section.getByLabel('生成数量', { exact: true }).isDisabled(), true);
    assert.equal(await section.getByLabel('生成数量', { exact: true }).inputValue(), '1');
    await section.getByLabel('每码可注册人数').fill('100');
    await section.getByRole('button', { name: '生成邀请码', exact: true }).click();
    const dialog = page.getByRole('dialog');
    await dialog.locator('textarea').fill('isolated UI regression');
    await dialog.getByRole('button', { name: /先预演|Run dry-run first/ }).click();
    await dialog.getByRole('button', { name: /确认执行|Confirm execution/ }).click();
    await dialog.getByText('测试成功').first().waitFor();
    assert.equal(lastAction.code, '01234');
    assert.equal(lastAction.count, 1);
    assert.equal(lastAction.max_uses, 100);
    await dialog.getByRole('button', { name: /^(关闭|Close)$/ }).last().click();
    await page.reload();
    await section.getByRole('button', {name: '复制邀请码 01234', exact: true}).waitFor();
    await section.getByText('SAF…（旧码未保存）', {exact: true}).waitFor();
    await section.screenshot({ path: '/tmp/safelink-registration-invites-desktop.png' });
    await page.setViewportSize({ width: 390, height: 844 });
    await section.scrollIntoViewIfNeeded();
    await page.waitForFunction(() => document.querySelector('.sidebar').getBoundingClientRect().right <= 0);
    await section.screenshot({ path: '/tmp/safelink-registration-invites-mobile.png' });
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth), false, 'page overflow');
    assert.deepEqual(errors, []);
    const policy = page.locator('#serversettings-login-policy');
    await policy.getByLabel('新用户必须设置两步验证密码').check();
    await policy.getByLabel('登录凭证有效期（天）').fill('90');
    await policy.getByRole('button', {name: '保存登录设置',exact:true}).click();
    await dialog.locator('textarea').fill('isolated login policy test');
    await dialog.getByRole('button', {name: /先预演|Run dry-run first/}).click();
    await dialog.getByRole('button', {name: /确认执行|Confirm execution/}).click();
    await dialog.getByText('登录设置测试成功').first().waitFor();
    assert.equal(lastAction.future_auth_days,90);
    assert.equal(lastAction.future_auth_enabled,true);
    assert.equal(lastAction.registration_password_required,true);
    await dialog.getByRole('button',{name:/^(关闭|Close)$/}).last().click();
    await policy.screenshot({path:'/tmp/safelink-login-policy-mobile.png'});
    await page.setViewportSize({width:1365,height:1000});
    await policy.screenshot({path:'/tmp/safelink-login-policy-desktop.png'});
    console.log('PASS: fixed/shared invite, limits, dry run, confirm, desktop/mobile layout');
  } finally { await browser.close(); }
})().catch(error => { console.error(error); process.exitCode = 1; });
