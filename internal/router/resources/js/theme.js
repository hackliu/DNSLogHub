/* DNSLogHUB 主题切换：暗色默认，明暗经 data-theme 切换并持久化。 */
(function () {
  var KEY = 'dnsloghub-theme';
  var theme = 'dark';
  try { theme = localStorage.getItem(KEY) || 'dark'; } catch (e) {}

  function apply(t) {
    document.documentElement.setAttribute('data-theme', t);
  }
  apply(theme);

  window.DNSLogHUB = window.DNSLogHUB || {};
  window.DNSLogHUB.getTheme = function () { return theme; };
  window.DNSLogHUB.toggleTheme = function () {
    theme = theme === 'dark' ? 'light' : 'dark';
    try { localStorage.setItem(KEY, theme); } catch (e) {}
    apply(theme);
    return theme;
  };
})();
