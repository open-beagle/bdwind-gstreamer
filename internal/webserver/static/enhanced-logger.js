/**
 * 增强日志记录器 - 统一错误处理格式和日志输出
 * 提供详细的状态变化日志记录和错误分类功能
 */
class EnhancedLogger {
  constructor() {
    this.logLevels = {
      DEBUG: 0,
      INFO: 1,
      WARN: 2,
      ERROR: 3,
      CRITICAL: 4
    };
    
    this.currentLevel = this.logLevels.INFO;
    this.maxEntries = 1000;
    this.logHistory = [];
    this.errorCategories = new Map();
    this.stateChangeHistory = [];
    
    // 日志格式配置
    this.config = {
      includeTimestamp: true,
      includeStackTrace: true,
      colorizeOutput: true,
      includeContext: true,
      maxContextDepth: 3
    };
    
    // 错误统计
    this.errorStats = {
      total: 0,
      byCategory: new Map(),
      byLevel: new Map(),
      recentErrors: []
    };
  }

  /**
   * 设置日志级别
   */
  setLevel(level) {
    if (typeof level === 'string') {
      this.currentLevel = this.logLevels[level.toUpperCase()] || this.logLevels.INFO;
    } else {
      this.currentLevel = level;
    }
  }

  /**
   * 统一的日志记录方法
   */
  log(level, category, message, data = {}, context = {}) {
    if (this.logLevels[level] < this.currentLevel) {
      return;
    }

    const timestamp = Date.now();
    const logEntry = this._formatLogEntry(level, category, message, data, context, timestamp);
    
    // 输出到控制台
    this._outputToConsole(logEntry);
    
    // 记录到历史
    this._recordToHistory(logEntry);
    
    // 更新统计
    this._updateStats(logEntry);
    
    return logEntry;
  }

  /**
   * 错误日志记录 - 包含错误分类和上下文信息收集
   */
  error(category, message, error, context = {}) {
    const errorInfo = this._analyzeError(error, category, context);
    
    return this.log('ERROR', category, message, {
      error: errorInfo,
      context: context
    });
  }

  /**
   * 警告日志记录
   */
  warn(category, message, data = {}, context = {}) {
    return this.log('WARN', category, message, data, context);
  }

  /**
   * 信息日志记录
   */
  info(category, message, data = {}, context = {}) {
    return this.log('INFO', category, message, data, context);
  }

  /**
   * 调试日志记录
   */
  debug(category, message, data = {}, context = {}) {
    return this.log('DEBUG', category, message, data, context);
  }

  /**
   * 状态变化日志记录 - 详细的状态变化跟踪
   */
  logStateChange(category, fromState, toState, context = {}) {
    const stateChange = {
      category: category,
      from: fromState,
      to: toState,
      timestamp: Date.now(),
      context: context,
      duration: context.duration || 0
    };

    console.log(`🔄 [状态变化] ${category}: ${fromState} → ${toState}`, {
      context: context,
      timestamp: new Date().toISOString(),
      duration: stateChange.duration
    });

    this.stateChangeHistory.push(stateChange);
    
    // 保持历史记录在合理范围内
    if (this.stateChangeHistory.length > 200) {
      this.stateChangeHistory = this.stateChangeHistory.slice(-100);
    }

    return stateChange;
  }

  /**
   * 连接事件日志记录
   */
  logConnectionEvent(eventType, details = {}, context = {}) {
    const connectionEvent = {
      type: eventType,
      details: details,
      context: context,
      timestamp: Date.now()
    };

    const emoji = this._getConnectionEmoji(eventType);
    console.log(`${emoji} [连接事件] ${eventType}`, {
      details: details,
      context: context,
      timestamp: new Date().toISOString()
    });

    return this.log('INFO', 'CONNECTION', eventType, connectionEvent);
  }

  /**
   * 性能日志记录
   */
  logPerformance(operation, duration, unit = 'ms', context = {}) {
    const performanceData = {
      operation: operation,
      duration: duration,
      unit: unit,
      context: context,
      timestamp: Date.now()
    };

    console.log(`⏱️ [性能] ${operation}: ${duration}${unit}`, {
      context: context,
      timestamp: new Date().toISOString()
    });

    return this.log('INFO', 'PERFORMANCE', operation, performanceData);
  }

  /**
   * 网络事件日志记录
   */
  logNetworkEvent(eventType, details = {}, context = {}) {
    const networkEvent = {
      type: eventType,
      details: details,
      context: context,
      timestamp: Date.now()
    };

    console.log(`🌐 [网络] ${eventType}`, {
      details: details,
      context: context,
      timestamp: new Date().toISOString()
    });

    return this.log('INFO', 'NETWORK', eventType, networkEvent);
  }

  /**
   * 格式化日志条目
   * @private
   */
  _formatLogEntry(level, category, message, data, context, timestamp) {
    return {
      level: level,
      category: category,
      message: message,
      data: data,
      context: context,
      timestamp: new Date(timestamp).toISOString(),
      timestampMs: timestamp
    };
  }

  /**
   * 输出到控制台
   * @private
   */
  _outputToConsole(logEntry) {
    const { level, category, message, data, context, timestamp } = logEntry;
    
    const prefix = this._getLevelPrefix(level);
    const categoryTag = `[${category}]`;
    
    const consoleMethod = this._getConsoleMethod(level);
    
    if (Object.keys(data).length > 0 || Object.keys(context).length > 0) {
      consoleMethod(`${prefix} ${categoryTag} ${message}`, {
        data: data,
        context: context,
        timestamp: timestamp
      });
    } else {
      consoleMethod(`${prefix} ${categoryTag} ${message}`);
    }
  }

  /**
   * 获取日志级别前缀
   * @private
   */
  _getLevelPrefix(level) {
    const prefixes = {
      DEBUG: '🔍',
      INFO: 'ℹ️',
      WARN: '⚠️',
      ERROR: '❌',
      CRITICAL: '🚨'
    };
    return prefixes[level] || 'ℹ️';
  }

  /**
   * 获取控制台方法
   * @private
   */
  _getConsoleMethod(level) {
    switch (level) {
      case 'DEBUG':
        return console.debug;
      case 'INFO':
        return console.log;
      case 'WARN':
        return console.warn;
      case 'ERROR':
      case 'CRITICAL':
        return console.error;
      default:
        return console.log;
    }
  }

  /**
   * 获取连接事件表情符号
   * @private
   */
  _getConnectionEmoji(eventType) {
    const emojiMap = {
      'connecting': '🔄',
      'connected': '✅',
      'disconnected': '❌',
      'reconnecting': '🔄',
      'failed': '💥',
      'timeout': '⏰',
      'ice-gathering': '🧊',
      'ice-connected': '🔗',
      'ice-failed': '❄️',
      'offer-received': '📥',
      'answer-sent': '📤',
      'candidate-received': '🎯',
      'media-received': '🎬'
    };
    return emojiMap[eventType] || '📡';
  }

  /**
   * 分析错误信息
   * @private
   */
  _analyzeError(error, category, context) {
    const errorObj = typeof error === 'string' ? new Error(error) : error;
    
    const errorInfo = {
      message: errorObj.message || '未知错误',
      name: errorObj.name || 'Error',
      stack: errorObj.stack,
      category: this._categorizeError(errorObj, category),
      context: context,
      timestamp: Date.now()
    };

    // 更新错误分类统计
    this._updateErrorCategory(errorInfo.category);

    return errorInfo;
  }

  /**
   * 错误分类
   * @private
   */
  _categorizeError(error, category) {
    const message = error.message?.toLowerCase() || '';
    
    // 基于错误消息的分类
    if (message.includes('network') || message.includes('fetch') || message.includes('timeout')) {
      return 'NETWORK_ERROR';
    } else if (message.includes('webrtc') || message.includes('ice') || message.includes('sdp')) {
      return 'WEBRTC_ERROR';
    } else if (message.includes('video') || message.includes('audio') || message.includes('media')) {
      return 'MEDIA_ERROR';
    } else if (message.includes('signaling') || message.includes('websocket')) {
      return 'SIGNALING_ERROR';
    } else if (category) {
      return `${category.toUpperCase()}_ERROR`;
    } else {
      return 'GENERAL_ERROR';
    }
  }

  /**
   * 更新错误分类统计
   * @private
   */
  _updateErrorCategory(category) {
    const count = this.errorCategories.get(category) || 0;
    this.errorCategories.set(category, count + 1);
  }

  /**
   * 记录到历史
   * @private
   */
  _recordToHistory(logEntry) {
    this.logHistory.push(logEntry);
    
    // 保持历史记录在合理范围内
    if (this.logHistory.length > this.maxEntries) {
      this.logHistory = this.logHistory.slice(-Math.floor(this.maxEntries * 0.8));
    }
  }

  /**
   * 更新统计信息
   * @private
   */
  _updateStats(logEntry) {
    if (logEntry.level === 'ERROR' || logEntry.level === 'CRITICAL') {
      this.errorStats.total++;
      
      const category = logEntry.data?.error?.category || 'UNKNOWN';
      const categoryCount = this.errorStats.byCategory.get(category) || 0;
      this.errorStats.byCategory.set(category, categoryCount + 1);
      
      const levelCount = this.errorStats.byLevel.get(logEntry.level) || 0;
      this.errorStats.byLevel.set(logEntry.level, levelCount + 1);
      
      this.errorStats.recentErrors.push({
        timestamp: logEntry.timestampMs,
        category: category,
        message: logEntry.message
      });
      
      // 保持最近错误记录在合理范围内
      if (this.errorStats.recentErrors.length > 50) {
        this.errorStats.recentErrors = this.errorStats.recentErrors.slice(-30);
      }
    }
  }

  /**
   * 获取错误统计信息
   */
  getErrorStats() {
    return {
      ...this.errorStats,
      byCategory: Object.fromEntries(this.errorStats.byCategory),
      byLevel: Object.fromEntries(this.errorStats.byLevel)
    };
  }

  /**
   * 获取状态变化历史
   */
  getStateChangeHistory(category = null, limit = 50) {
    let history = this.stateChangeHistory;
    
    if (category) {
      history = history.filter(change => change.category === category);
    }
    
    return history.slice(-limit);
  }

  /**
   * 获取日志历史
   */
  getLogHistory(level = null, category = null, limit = 100) {
    let history = this.logHistory;
    
    if (level) {
      history = history.filter(entry => entry.level === level);
    }
    
    if (category) {
      history = history.filter(entry => entry.category === category);
    }
    
    return history.slice(-limit);
  }

  /**
   * 清理历史记录
   */
  clearHistory() {
    this.logHistory = [];
    this.stateChangeHistory = [];
    this.errorStats = {
      total: 0,
      byCategory: new Map(),
      byLevel: new Map(),
      recentErrors: []
    };
    this.errorCategories.clear();
  }

  /**
   * 导出日志数据
   */
  exportLogs() {
    return {
      config: this.config,
      logHistory: this.logHistory,
      stateChangeHistory: this.stateChangeHistory,
      errorStats: this.getErrorStats(),
      errorCategories: Object.fromEntries(this.errorCategories),
      exportTimestamp: new Date().toISOString()
    };
  }
}

// 创建全局增强日志记录器实例
window.EnhancedLogger = window.EnhancedLogger || new EnhancedLogger();