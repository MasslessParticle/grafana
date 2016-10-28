///<reference path="../../../headers/common.d.ts" />

import _ from 'lodash';
import $ from 'jquery';
import angular from 'angular';

import config from 'app/core/config';
import {coreModule} from 'app/core/core';

import './options';
import './add_panel';

export class DashRowCtrl {
  dashboard: any;
  row: any;
  dropView: number;

  /** @ngInject */
  constructor(private $scope, private $rootScope, private $timeout, private uiSegmentSrv, private $q) {
    this.row.title = this.row.title || 'Row title';

    if (this.dashboard.meta.isNew) {
      this.dropView = 1;
      delete this.row.isNew;
    }
  }

  onDrop(panelId, dropTarget) {
    var info = this.dashboard.getPanelInfoById(panelId);
    if (dropTarget) {
      var dropInfo = this.dashboard.getPanelInfoById(dropTarget.id);
      dropInfo.row.panels[dropInfo.index] = info.panel;
      info.row.panels[info.index] = dropTarget;
      var dragSpan = info.panel.span;
      info.panel.span = dropTarget.span;
      dropTarget.span = dragSpan;
    } else {
      info.row.panels.splice(info.index, 1);
      info.panel.span = 12 - this.dashboard.rowSpan(this.row);
      this.row.panels.push(info.panel);
    }

    this.$rootScope.$broadcast('render');
  }

  setHeight(height) {
    this.row.height = height;
    this.$scope.$broadcast('render');
  }

  moveRow(direction) {
    var rowsList = this.dashboard.rows;
    var rowIndex = _.indexOf(rowsList, this.row);
    var newIndex = rowIndex;
    switch (direction) {
      case 'up': {
        newIndex = rowIndex - 1;
        break;
      }
      case 'down': {
        newIndex = rowIndex + 1;
        break;
      }
      case 'top': {
        newIndex = 0;
        break;
      }
      case 'bottom': {
        newIndex = rowsList.length - 1;
        break;
      }
      default: {
        newIndex = rowIndex;
      }
    }
    if (newIndex >= 0 && newIndex <= (rowsList.length - 1)) {
      _.move(rowsList, rowIndex, newIndex);
    }
  }

  toggleCollapse() {
    this.dropView = 0;
    this.row.collapse = !this.row.collapse;
  }

  showAddPanel() {
    this.dropView = this.dropView === 1 ? 0 : 1;
  }

  showRowOptions() {
    this.dropView = this.dropView === 2 ? 0 : 2;
  }
}

export function rowDirective($rootScope) {
  return {
    restrict: 'E',
    templateUrl: 'public/app/features/dashboard/row/row.html',
    controller: DashRowCtrl,
    bindToController: true,
    controllerAs: 'ctrl',
    scope: {
      dashboard: "=",
      row: "=",
    },
    link: function(scope, element) {
      scope.$watchGroup(['ctrl.row.collapse', 'ctrl.row.height', 'ctrl.row.showTitle', 'ctrl.dropView'], function() {
        element.css({minHeight: scope.ctrl.row.collapse ? '5px' : scope.ctrl.row.height});
        element.toggleClass('dash-row-show-title', scope.ctrl.row.showTitle === true);
        element.toggleClass('dash-row-show-options', scope.ctrl.dropView === 2);
        element.toggleClass('dash-row-show-add-panel', scope.ctrl.dropView === 1);
      });

      $rootScope.onAppEvent('panel-fullscreen-enter', function(evt, info) {
        var hasPanel = _.find(scope.ctrl.row.panels, {id: info.panelId});
        if (!hasPanel) {
          element.hide();
        }
      }, scope);

      $rootScope.onAppEvent('panel-fullscreen-exit', function() {
        element.show();
      }, scope);
    }
  };
}

coreModule.directive('dashRow', rowDirective);


coreModule.directive('panelWidth', function($rootScope) {
  return function(scope, element) {
    var fullscreen = false;

    function updateWidth() {
      if (!fullscreen) {
        element[0].style.width = ((scope.panel.span / 1.2) * 10) + '%';
      }
    }

    $rootScope.onAppEvent('panel-fullscreen-enter', function(evt, info) {
      fullscreen = true;

      if (scope.panel.id !== info.panelId) {
        element.hide();
      } else {
        element[0].style.width = '100%';
      }
    }, scope);

    $rootScope.onAppEvent('panel-fullscreen-exit', function(evt, info) {
      fullscreen = false;

      if (scope.panel.id !== info.panelId) {
        element.show();
      }

      updateWidth();
    }, scope);

    scope.$watch('panel.span', updateWidth);

    if (fullscreen) {
      element.hide();
    }
  };
});


coreModule.directive('panelDropZone', function($timeout) {
  return function(scope, element) {
    scope.$on("ANGULAR_DRAG_START", function() {
      $timeout(function() {
        var dropZoneSpan = 12 - scope.ctrl.dashboard.rowSpan(scope.ctrl.row);

        if (dropZoneSpan > 0) {
          element.find('.panel-container').css('height', scope.ctrl.row.height);
          element[0].style.width = ((dropZoneSpan / 1.2) * 10) + '%';
          element.show();
        }
      });
    });

    scope.$on("ANGULAR_DRAG_END", function() {
      element.hide();
    });
  };
});

