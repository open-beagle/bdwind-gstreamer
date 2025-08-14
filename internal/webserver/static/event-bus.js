/**
 * EventBus - 事件总线系统
 * 提供模块间的解耦通信机制
 */
class EventBus {
    constructor() {
        this._events = new Map();
        this._onceEvents = new Set();
    }

    /**
     * 订阅事件
     * @param {string} eventName - 事件名称
     * @param {Function} callback - 回调函数
     * @param {Object} context - 执行上下文
     * @returns {Function} 取消订阅函数
     */
    on(eventName, callback, context = null) {
        if (typeof eventName !== 'string' || typeof callback !== 'function') {
            throw new Error('EventBus.on: eventName must be string and callback must be function');
        }

        if (!this._events.has(eventName)) {
            this._events.set(eventName, []);
        }

        const listener = { callback, context };
        this._events.get(eventName).push(listener);

        // 返回取消订阅函数
        return () => this.off(eventName, callback, context);
    }

    /**
     * 取消订阅事件
     * @param {string} eventName - 事件名称
     * @param {Function} callback - 回调函数
     * @param {Object} context - 执行上下文
     */
    off(eventName, callback, context = null) {
        if (!this._events.has(eventName)) {
            return;
        }

        const listeners = this._events.get(eventName);
        const index = listeners.findIndex(listener => 
            listener.callback === callback && listener.context === context
        );

        if (index !== -1) {
            listeners.splice(index, 1);
            
            // 如果没有监听器了，删除事件
            if (listeners.length === 0) {
                this._events.delete(eventName);
            }
        }
    }

    /**
     * 发布事件
     * @param {string} eventName - 事件名称
     * @param {*} data - 事件数据
     */
    emit(eventName, data = null) {
        if (!this._events.has(eventName)) {
            return;
        }

        const listeners = this._events.get(eventName).slice(); // 复制数组避免在遍历时修改
        
        listeners.forEach(listener => {
            try {
                if (listener.context) {
                    listener.callback.call(listener.context, data);
                } else {
                    listener.callback(data);
                }
            } catch (error) {
                console.error(`EventBus: Error in event handler for "${eventName}":`, error);
            }
        });

        // 清理一次性事件监听器
        this._cleanupOnceListeners(eventName);
    }

    /**
     * 一次性订阅事件
     * @param {string} eventName - 事件名称
     * @param {Function} callback - 回调函数
     * @param {Object} context - 执行上下文
     * @returns {Function} 取消订阅函数
     */
    once(eventName, callback, context = null) {
        const onceCallback = (data) => {
            callback.call(context, data);
            this.off(eventName, onceCallback, context);
        };

        // 标记为一次性监听器
        this._onceEvents.add(onceCallback);
        
        return this.on(eventName, onceCallback, context);
    }

    /**
     * 清理所有事件监听器
     */
    clear() {
        this._events.clear();
        this._onceEvents.clear();
    }

    /**
     * 获取事件监听器数量
     * @param {string} eventName - 事件名称（可选）
     * @returns {number} 监听器数量
     */
    getListenerCount(eventName = null) {
        if (eventName) {
            return this._events.has(eventName) ? this._events.get(eventName).length : 0;
        }
        
        let total = 0;
        for (const listeners of this._events.values()) {
            total += listeners.length;
        }
        return total;
    }

    /**
     * 获取所有事件名称
     * @returns {string[]} 事件名称数组
     */
    getEventNames() {
        return Array.from(this._events.keys());
    }

    /**
     * 检查是否有指定事件的监听器
     * @param {string} eventName - 事件名称
     * @returns {boolean} 是否有监听器
     */
    hasListeners(eventName) {
        return this._events.has(eventName) && this._events.get(eventName).length > 0;
    }

    /**
     * 清理一次性事件监听器
     * @private
     */
    _cleanupOnceListeners(eventName) {
        if (!this._events.has(eventName)) {
            return;
        }

        const listeners = this._events.get(eventName);
        const filteredListeners = listeners.filter(listener => !this._onceEvents.has(listener.callback));
        
        if (filteredListeners.length !== listeners.length) {
            if (filteredListeners.length === 0) {
                this._events.delete(eventName);
            } else {
                this._events.set(eventName, filteredListeners);
            }
        }
    }
}

// 导出EventBus类
if (typeof module !== 'undefined' && module.exports) {
    module.exports = EventBus;
} else if (typeof window !== 'undefined') {
    window.EventBus = EventBus;
}