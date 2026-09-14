import React, { FormEvent, useEffect, useState } from 'react';
import { createRoot } from 'react-dom/client';
import i18n from 'i18next';
import { initReactI18next, useTranslation } from 'react-i18next';
import './style.css';

void i18n.use(initReactI18next).init({
  lng: 'en-GB', fallbackLng: 'en-GB', interpolation: { escapeValue: false },
  resources: {
    'en-GB': { translation: {
      title: 'Your team. In the right place.',
      description: 'Verified attendance, working hours and payments for field teams.',
      signIn: 'Sign in', phone: 'Phone number', code: 'Verification code', requestCode: 'Send code', verifyCode: 'Verify and sign in', signOut: 'Sign out',
      codeSent: 'Code sent. In local development, find it in the API logs.', signedInAs: 'Signed in as', requestFailed: 'We could not send a code. Check the phone number and try again.', verifyFailed: 'The code is invalid or has expired. Request a new one.',
      language: 'Language',
    } },
    'pt-BR': { translation: {
      title: 'Sua equipe. No lugar certo.',
      description: 'Presença verificada, horas trabalhadas e pagamentos para equipes externas.',
      signIn: 'Entrar', phone: 'Número de telefone', code: 'Código de verificação', requestCode: 'Enviar código', verifyCode: 'Verificar e entrar', signOut: 'Sair',
      codeSent: 'Código enviado. No desenvolvimento local, consulte os logs da API.', signedInAs: 'Sessão iniciada como', requestFailed: 'Não foi possível enviar o código. Confira o telefone e tente novamente.', verifyFailed: 'O código é inválido ou expirou. Solicite um novo.',
      language: 'Idioma',
    } },
  },
});

type User = { id: string; organizationId: string; name: string; phone: string; role: string };

function App() {
  const { t } = useTranslation();
  const [phone, setPhone] = useState('');
  const [code, setCode] = useState('');
  const [sent, setSent] = useState(false);
  const [user, setUser] = useState<User | null>(null);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  useEffect(() => { void fetch('/api/me').then(async response => { if (response.ok) setUser((await response.json() as { user: User }).user); }); }, []);
  async function requestCode(event: FormEvent) { event.preventDefault(); setLoading(true); setError(''); const response = await fetch('/api/auth/request-otp', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ phone }) }); setLoading(false); if (response.ok) setSent(true); else setError(t('requestFailed')); }
  async function verifyCode(event: FormEvent) { event.preventDefault(); setLoading(true); setError(''); const response = await fetch('/api/auth/verify-otp', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ phone, code }) }); setLoading(false); if (response.ok) setUser((await response.json() as { user: User }).user); else setError(t('verifyFailed')); }
  async function signOut() { await fetch('/api/auth/logout', { method: 'POST' }); setUser(null); setCode(''); setSent(false); }
  return <main>
    <header><strong>WorkPin</strong><label>{t('language')}{' '}
      <select value={i18n.language} onChange={event => {
        const language = event.target.value;
        void i18n.changeLanguage(language);
        document.documentElement.lang = language;
      }}><option value="en-GB">English</option><option value="pt-BR">Português</option></select>
    </label></header>
    <section><h1>{t('title')}</h1><p>{t('description')}</p>
      <aside>{user ? <><p>{t('signedInAs')} <strong>{user.name}</strong></p><p>{user.role}</p><button onClick={() => void signOut()}>{t('signOut')}</button></> : <form onSubmit={sent ? verifyCode : requestCode}><h2>{t('signIn')}</h2><label>{t('phone')}<input required autoComplete="tel" inputMode="tel" value={phone} onChange={event => setPhone(event.target.value)} placeholder="+447700900123" /></label>{sent && <><p>{t('codeSent')}</p><label>{t('code')}<input required autoComplete="one-time-code" inputMode="numeric" pattern="[0-9]{6}" value={code} onChange={event => setCode(event.target.value)} /></label></>}{error && <p role="alert">{error}</p>}<button disabled={loading} type="submit">{sent ? t('verifyCode') : t('requestCode')}</button></form>}</aside>
    </section>
  </main>;
}

createRoot(document.getElementById('root')!).render(<React.StrictMode><App /></React.StrictMode>);
