import React, { useState, useEffect } from 'react';
import './App.css';

function App() {
  const [image, setImage] = useState(null);
  const [previewUrl, setPreviewUrl] = useState(null);
  const [isAnalyzing, setIsAnalyzing] = useState(false);
  const [result, setResult] = useState(null);
  const [theme, setTheme] = useState('light');
  const [darkModeOverride, setDarkModeOverride] = useState(false);

  useEffect(() => {
    const updateTheme = () => {
      if (window.Telegram && window.Telegram.WebApp) {
        const tgTheme = window.Telegram.WebApp.colorScheme;
        setTheme(tgTheme === 'dark' ? 'dark' : 'light');
      } else {
        const systemPrefersDark = window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches;
        setTheme(systemPrefersDark ? 'dark' : 'light');
      }
    };

    updateTheme();
    const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)');
    mediaQuery.addEventListener('change', updateTheme);
    return () => mediaQuery.removeEventListener('change', updateTheme);
  }, []);

  useEffect(() => {
    const effectiveTheme = darkModeOverride ? (theme === 'dark' ? 'light' : 'dark') : theme;
    document.body.className = `app ${effectiveTheme}-theme`;
  }, [theme, darkModeOverride]);

  const handleImageChange = (e) => {
    const file = e.target.files[0];
    if (file && file.type.startsWith('image/')) {
      setImage(file);
      setPreviewUrl(URL.createObjectURL(file));
      setResult(null);
    }
  };

  const toggleTheme = () => {
    setDarkModeOverride(!darkModeOverride);
  };

  const handleAnalyze = async () => {
    if (!image) return;

    setIsAnalyzing(true);
    setResult(null);

    const formData = new FormData();
    formData.append('image', image);

    try {
      const response = await fetch('/api/analyze', {
        method: 'POST',
        body: formData,
      });

      if (response.ok) {
        const data = await response.json();
        setResult({
          isDeepfake: data.is_deepfake,
          confidence: data.confidence,
        });
      } else {
        alert('Ошибка при анализе изображения.');
      }
    } catch (error) {
      console.error('Network error:', error);
      alert('Произошла ошибка соединения.');
    } finally {
      setIsAnalyzing(false);
    }
  };

  const openAbout = () => {
    alert("Мы — команда студентов, создающих приложение для обнаружения deepfake изображений. Наша цель — помочь людям распознавать фейковый контент.");
  };

  return (
    <div className="container">
      <header className="header">
        <h1 className="logo">
          <span className="logo-icon">🕵️</span>
          DeepDetect
        </h1>
        <div className="header-actions">
          <button onClick={openAbout} className="btn-ghost">О нас</button>
          <button onClick={toggleTheme} className="btn-ghost theme-toggle">
            {darkModeOverride ? (theme === 'dark' ? '☀️' : '🌙') : '🌓'}
          </button>
        </div>
      </header>

      <main className="main-content">
        <div className="upload-card">
          {previewUrl ? (
            <div className="preview-container">
              <img src={previewUrl} alt="Preview" className="image-preview" />
              <button className="btn-change" onClick={() => { setImage(null); setPreviewUrl(null); }}>
                ✕
              </button>
            </div>
          ) : (
            <>
              <input
                type="file"
                id="image-upload"
                accept="image/*"
                onChange={handleImageChange}
                style={{ display: 'none' }}
              />
              <label htmlFor="image-upload" className="upload-label">
                <div className="upload-icon">📁</div>
                <div className="upload-text">Выберите изображение</div>
                <div className="upload-hint">или перетащите сюда</div>
              </label>
            </>
          )}

          <button
            onClick={handleAnalyze}
            disabled={!image || isAnalyzing}
            className="analyze-btn"
          >
            {isAnalyzing ? (
              <>
                <span className="spinner"></span>
                Анализ...
              </>
            ) : (
              'Анализировать'
            )}
          </button>
        </div>

        {isAnalyzing && (
          <div className="analysis-progress">
            <div className="progress-bar">
              <div className="progress-fill"></div>
            </div>
            <p>Анализируем изображение...</p>
          </div>
        )}

        {result && (
          <div className={`result-card ${result.isDeepfake ? 'deepfake' : 'real'}`}>
            <div className="result-icon">
              {result.isDeepfake ? '⚠️' : '✅'}
            </div>
            <h2>{result.isDeepfake ? 'Обнаружен deepfake' : 'Изображение подлинное'}</h2>
            <div className="confidence-meter">
              <div
                className="confidence-fill"
                style={{ width: `${result.confidence * 100}%` }}
              ></div>
            </div>
            <p className="confidence-text">
              Уверенность: {(result.confidence * 100).toFixed(1)}%
            </p>
          </div>
        )}
      </main>
    </div>
  );
}

export default App;