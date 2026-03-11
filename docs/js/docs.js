// Mobile sidebar toggle
const sidebarToggle = document.getElementById('sidebar-toggle');
const sidebar = document.getElementById('sidebar');
const sidebarOverlay = document.getElementById('sidebar-overlay');

if (sidebarToggle && sidebar) {
  sidebarToggle.addEventListener('click', () => {
    sidebar.classList.toggle('-translate-x-full');
    if (sidebarOverlay) sidebarOverlay.classList.toggle('hidden');
  });
}
if (sidebarOverlay) {
  sidebarOverlay.addEventListener('click', () => {
    sidebar.classList.add('-translate-x-full');
    sidebarOverlay.classList.add('hidden');
  });
}

// Mobile nav toggle
const navToggle = document.getElementById('nav-toggle');
const mobileMenu = document.getElementById('mobile-menu');

if (navToggle && mobileMenu) {
  navToggle.addEventListener('click', () => {
    mobileMenu.classList.toggle('hidden');
  });
}

// Active sidebar link highlighting
const currentPath = window.location.pathname.replace(/\/$/, '') || '/';
const currentHash = window.location.hash;

document.querySelectorAll('#sidebar a').forEach(link => {
  const href = link.getAttribute('href');
  if (!href) return;

  const [linkPath, linkHash] = href.split('#');
  const normalizedLinkPath = linkPath.replace(/\/$/, '') || '/';

  if (currentHash && linkHash && normalizedLinkPath === currentPath && '#' + linkHash === currentHash) {
    link.classList.add('bg-feather-50', 'text-feather-700', 'font-medium');
  } else if (!linkHash && normalizedLinkPath === currentPath) {
    link.classList.add('bg-feather-50', 'text-feather-700', 'font-medium');
  }
});

// Smooth scroll for anchor links
document.querySelectorAll('a[href^="#"]').forEach(link => {
  link.addEventListener('click', (e) => {
    const target = document.querySelector(link.getAttribute('href'));
    if (target) {
      e.preventDefault();
      target.scrollIntoView({ behavior: 'smooth', block: 'start' });
      history.pushState(null, '', link.getAttribute('href'));
    }
  });
});

// Update active sidebar link on scroll (for anchor links)
const observer = new IntersectionObserver((entries) => {
  entries.forEach(entry => {
    if (entry.isIntersecting) {
      const id = entry.target.id;
      document.querySelectorAll('#sidebar a').forEach(link => {
        const href = link.getAttribute('href');
        if (href && href.includes('#' + id)) {
          link.classList.add('bg-feather-50', 'text-feather-700', 'font-medium');
        } else if (href && href.includes('#')) {
          link.classList.remove('bg-feather-50', 'text-feather-700', 'font-medium');
        }
      });
    }
  });
}, { rootMargin: '-80px 0px -80% 0px' });

document.querySelectorAll('h2[id], h3[id]').forEach(heading => {
  observer.observe(heading);
});
