export function useFormatters() {
  // Format date for display
  const formatDate = (dateString: any) => {
    console.log(dateString);
    
    const date = new Date(dateString);
    const locale = window.navigator.languages ? window.navigator.languages[0] : (window.navigator as any).userLanguage || window.navigator.language;
    return date.toLocaleDateString(locale, {
      year: 'numeric',
      month: 'long',
      day: 'numeric'
    });
  };

  
  // Convert nanoseconds to hours
  const convertNanoToHours = (nanoseconds: number) => {
    return (nanoseconds / 1e9 / 3600).toFixed(0);
  };


  // Calculate time since publication
  const timeAgo = (dateString: any) => {
    const date = new Date(dateString);
    const now = new Date();
    const seconds = Math.floor((now.getTime() - date.getTime()) / 1000);

    let interval = Math.floor(seconds / 31536000);
    if (interval > 1) {
      return `${interval} years ago`;
    }
    if (interval === 1) {
      return 'over 1 year ago';
    }

    interval = Math.floor(seconds / 2592000);
    if (interval > 1) {
      return `${interval} months ago`;
    }
    if (interval === 1) {
      return 'over 1 month ago';
    }

    interval = Math.floor(seconds / 86400);
    if (interval > 1) {
      return `${interval} days ago`;
    }
    if (interval === 1) {
      return 'yesterday';
    }

    interval = Math.floor(seconds / 3600);
    if (interval > 1) {
      return `${interval} hours ago`;
    }
    if (interval === 1) {
      return 'an hour ago';
    }

    interval = Math.floor(seconds / 60);
    if (interval > 1) {
      return `${interval} minutes ago`;
    }
    if (interval === 1) {
      return 'a minute ago';
    }

    return 'just now';
  };

  return {
    formatDate,
    convertNanoToHours,
    timeAgo
  };
}