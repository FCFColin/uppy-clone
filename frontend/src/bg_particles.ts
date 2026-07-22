export function initBgParticles(): void {
  const container = document.getElementById('bg-particles');
  if (!container) return;
  const canvas = document.createElement('canvas');
  canvas.style.width = '100%';
  canvas.style.height = '100%';
  container.appendChild(canvas);
  const ctxRaw = canvas.getContext('2d');
  if (!ctxRaw) return;
  const ctx: CanvasRenderingContext2D = ctxRaw;

  const prefersReducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  const isMobile = window.innerWidth < 768;

  let w = 0, h = 0, dpr = 1;
  let particles: Particle[] = [];
  let animationId: number;
  let time = 0;

  interface Particle {
    x: number; y: number;
    baseX: number; baseY: number;
    vx: number; vy: number;
    r: number;
    baseAlpha: number;
    alpha: number;
    color: string;
    twinklePhase: number;
    twinkleSpeed: number;
    driftPhaseX: number;
    driftPhaseY: number;
    driftAmpX: number;
    driftAmpY: number;
  }

  const colors: readonly string[] = [
    'rgba(200, 220, 255, 1)',
    'rgba(180, 200, 255, 1)',
    'rgba(220, 200, 255, 1)',
    'rgba(255, 255, 255, 1)',
    'rgba(180, 210, 255, 1)',
  ];

  function initParticles(): void {
    particles = [];
    let count: number;
    if (prefersReducedMotion) {
      count = isMobile ? 15 : 30;
    } else {
      count = isMobile ? 40 : 80;
    }

    for (let i = 0; i < count; i++) {
      const x = Math.random() * w;
      const y = Math.random() * h;
      particles.push({
        x,
        y,
        baseX: x,
        baseY: y,
        vx: (Math.random() - 0.5) * 0.02 * dpr,
        vy: (Math.random() - 0.5) * 0.015 * dpr,
        r: (Math.random() * 2 + 0.5) * dpr,
        baseAlpha: Math.random() * 0.5 + 0.1,
        alpha: Math.random() * 0.5 + 0.1,
        color: colors[Math.floor(Math.random() * colors.length)] ?? 'rgba(255,255,255,1)',
        twinklePhase: Math.random() * Math.PI * 2,
        twinkleSpeed: Math.random() * 0.008 + 0.002,
        driftPhaseX: Math.random() * Math.PI * 2,
        driftPhaseY: Math.random() * Math.PI * 2,
        driftAmpX: (Math.random() * 0.5 + 0.2) * dpr,
        driftAmpY: (Math.random() * 0.5 + 0.2) * dpr,
      });
    }
  }

  function resize(): void {
    dpr = window.devicePixelRatio || 1;
    w = canvas.width = window.innerWidth * dpr;
    h = canvas.height = window.innerHeight * dpr;
    canvas.style.width = window.innerWidth + 'px';
    canvas.style.height = window.innerHeight + 'px';
    initParticles();
  }

  resize();
  window.addEventListener('resize', resize);

  function animate(): void {
    ctx.clearRect(0, 0, w, h);
    time += 1;

    for (const p of particles) {
      if (!prefersReducedMotion) {
        p.baseX += p.vx;
        p.baseY += p.vy;

        p.x = p.baseX + Math.sin(time * 0.003 + p.driftPhaseX) * p.driftAmpX;
        p.y = p.baseY + Math.cos(time * 0.0025 + p.driftPhaseY) * p.driftAmpY;

        p.twinklePhase += p.twinkleSpeed;
        p.alpha = p.baseAlpha * (0.6 + 0.4 * Math.sin(p.twinklePhase));

        if (p.baseX < -10 * dpr) p.baseX = w + 10 * dpr;
        if (p.baseX > w + 10 * dpr) p.baseX = -10 * dpr;
        if (p.baseY < -10 * dpr) p.baseY = h + 10 * dpr;
        if (p.baseY > h + 10 * dpr) p.baseY = -10 * dpr;
      } else {
        p.x = p.baseX;
        p.y = p.baseY;
        p.alpha = p.baseAlpha;
      }

      ctx.beginPath();
      ctx.arc(p.x, p.y, p.r, 0, Math.PI * 2);
      ctx.fillStyle = p.color;
      ctx.globalAlpha = p.alpha;
      ctx.shadowBlur = p.r * 3;
      ctx.shadowColor = p.color;
      ctx.fill();
    }

    ctx.globalAlpha = 1;
    ctx.shadowBlur = 0;
    animationId = requestAnimationFrame(animate);
  }

  animate();

  window.addEventListener('beforeunload', () => {
    if (animationId) {
      cancelAnimationFrame(animationId);
    }
  });
}
