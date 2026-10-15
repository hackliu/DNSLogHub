/* DNSLogHUB 轻量 i18n：字典由各页面 window.DNSLOGHUB_DICT 提供，
   通过 data-i18n / data-i18n-placeholder / data-i18n-title 属性替换文案。 */
(function () {
  var KEY = 'dnsloghub-lang';
  var lang = 'zh-CN';
  try { lang = localStorage.getItem(KEY) || 'zh-CN'; } catch (e) {}

  function dict() { return window.DNSLOGHUB_DICT || {}; }

  function t(key) {
    var d = dict();
    var m = d[lang] || d['zh-CN'] || {};
    return m[key] != null ? m[key] : key;
  }

  function apply() {
    document.querySelectorAll('[data-i18n]').forEach(function (el) {
      el.textContent = t(el.getAttribute('data-i18n'));
    });
    document.querySelectorAll('[data-i18n-placeholder]').forEach(function (el) {
      el.setAttribute('placeholder', t(el.getAttribute('data-i18n-placeholder')));
    });
    document.querySelectorAll('[data-i18n-title]').forEach(function (el) {
      el.setAttribute('title', t(el.getAttribute('data-i18n-title')));
    });
    document.documentElement.lang = lang === 'en' ? 'en' : 'zh-CN';
  }

  window.I18N = {
    t: t,
    lang: function () { return lang; },
    setLang: function (l) {
      lang = l;
      try { localStorage.setItem(KEY, l); } catch (e) {}
      apply();
    },
    apply: apply
  };

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', apply);
  } else {
    apply();
  }
})();
